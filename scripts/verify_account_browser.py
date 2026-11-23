"""对隔离真实应用执行账号安全浏览器验收，受控界面响应与真实后端证据分别计数。"""

from __future__ import annotations

import sys

if __name__ == '__main__':
    sys.dont_write_bytecode = True

import argparse
from contextlib import closing
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shutil
from typing import Any

from playwright.sync_api import Browser, BrowserContext, Error as PlaywrightError, Locator, Page, Playwright, Request, Route, expect, sync_playwright

from verify_account import AccountHarness
from verify_baseline import Blocked, file_identity, new_run_root, write_json


def frontend_identity(web: Path) -> dict[str, Any]:
    """为实际复制的首页和全部构建资产记录相对路径、大小及内容摘要。"""
    paths: list[Path] = [web / 'index.html']
    assets: Path = web / 'assets'
    if not assets.is_dir() or assets.is_symlink() or getattr(assets.lstat(), 'st_file_attributes', 0) & 1024:
        raise Blocked('缺少前端构建资产目录')
    for path in sorted(assets.rglob('*')):
        if path.is_symlink() or getattr(path.lstat(), 'st_file_attributes', 0) & 1024:
            raise Blocked('前端构建资产不能经过链接或重解析点')
        if path.is_file():
            paths.append(path)
    files: list[dict[str, Any]] = []
    for path in paths:
        identity: dict[str, Any] = file_identity(path)
        files.append({'path': path.relative_to(web).as_posix(), 'bytes': identity['bytes'], 'sha256': identity['sha256']})
    if len(files) < 2:
        raise Blocked('前端构建资产为空')
    return {'files': files, 'sha256': hashlib.sha256(json.dumps(files, sort_keys=True, separators=(',', ':')).encode()).hexdigest()}


class AccountBrowserChecks:
    """只保留安全断言、分类结果与无凭据截图，不保存请求正文或浏览器原始异常。"""

    def __init__(self, harness: AccountHarness, root: Path, password: str) -> None:
        """绑定自有实例和内存凭据，隔离真实行为与界面夹具证据。"""
        self.harness: AccountHarness = harness  # 仅由本脚本启动的隔离服务。
        self.root: Path = root  # 已通过基线路径检查的唯一证据根。
        self.password: str = password  # 随机初始密码只在内存使用。
        self.username: str = 'account-browser'  # 仅隔离实例的公开账号名。
        self.assertions: list[dict[str, str]] = []  # 已完成的安全中文断言及证据类型。
        self.stage: str = '初始化'  # 出错时只发布固定阶段名。
        self.secrets: list[str] = [password]  # 密码、真实Cookie及摘要仅用于内存泄露检查。
        self.console: list[str] = []  # 原始控制台内容只在内存检查，绝不写日志。
        self.console_overflow: bool = False  # 超过有界预算时不能声称完整扫描。
        self.page_error_count: int = 0  # 不持久化可能含密码的异常正文。
        self.mutations: list[str] = []  # 仅记录账号写请求方法及路径，不记录正文。
        self.screenshots: list[str] = []  # 密码清空后保存的相对图片路径。
        self.cleanup_failed: bool = False  # 任一浏览器上下文关闭失败必须使结果失败。

    def check(self, condition: bool, label: str, kind: str = 'real-browser') -> None:
        """记录实际完成的断言，固定说明禁止包含凭据或响应正文。"""
        if not condition:
            raise AssertionError(label)
        self.assertions.append({'kind': kind, 'label': label})
        print('  ✓ ' + label, flush=True)

    def observe(self, page: Page) -> None:
        """绑定有界内存事件观察器，不把浏览器错误或请求正文打印到终端。"""
        def page_error(_: Any) -> None:
            """仅统计页面未处理异常数量。"""
            self.page_error_count += 1

        def console_message(message: Any) -> None:
            """有界保存原始控制台消息供秘密检查，超限明确失败。"""
            text: str = message.text
            if len(self.console) >= 200 or len(text) > 16384:
                self.console_overflow = True
                return
            self.console.append(text)

        def request(request: Request) -> None:
            """账号写操作只计数，不保存包含密码的请求体。"""
            if '/api/account/' in request.url and request.method != 'GET':
                self.mutations.append(request.method + ' ' + request.url.split('/api/account/', 1)[1].split('?', 1)[0])

        page.on('pageerror', page_error)
        page.on('console', console_message)
        page.on('request', request)

    def login(self, page: Page, password: str) -> None:
        """用实际登录表单建立隔离浏览器会话，不打印密码或HTTP错误正文。"""
        self.secrets.append(password)
        page.goto(self.harness.client.base)
        page.get_by_label('用户名', exact=True).fill(self.username)
        page.get_by_label('密码', exact=True).fill(password)
        page.get_by_role('button', name='进入内容空间', exact=True).click()
        expect(page.locator('.app-shell')).to_be_visible()
        self.remember_cookies(page.context)

    def settings(self, page: Page) -> Locator:
        """通过现有路由进入局部账号安全组件。"""
        page.evaluate("location.hash = '#settings'")
        section = page.locator('section[aria-labelledby="account-security-title"]')
        expect(section).to_be_visible()
        return section

    def clear_fields(self, page: Page) -> None:
        """在截图和状态切换前清空可见账号密码字段。"""
        for identity in ('current-password', 'new-password', 'confirm-password', 'revoke-password'):
            field = page.locator('#account-' + identity)
            if field.count() and field.is_enabled():
                field.fill('')

    def cleared(self, page: Page, label: str, kind: str = 'real-browser') -> None:
        """断言四个敏感字段清空并恢复默认掩码。"""
        fields = page.locator('input[id^="account-"][id$="-password"]')
        expect(fields).to_have_count(4)
        for field in fields.all():
            expect(field).to_have_value('')
            expect(field).to_have_attribute('type', 'password')
        self.check(fields.count() == 4 and all(field.input_value() == '' and field.get_attribute('type') == 'password' for field in fields.all()), label, kind)

    def remember_cookies(self, context: BrowserContext) -> None:
        """只在内存记录实际bearer及SHA摘要以检查DTO和浏览器存储。"""
        for cookie in context.cookies():
            if cookie['name'] == 'nas_session':
                self.remember_session_token(cookie['value'])

    def remember_session_token(self, token: str) -> None:
        """将每次真实登录的Cookie及摘要加入内存扫描，撤销后也不丢弃。"""
        self.secrets.extend((token, hashlib.sha256(token.encode()).hexdigest()))

    def remember_database_secrets(self) -> None:
        """采集隔离数据库当前密码摘要与会话摘要，覆盖改密前后的不同值。"""
        with closing(self.harness.database()) as database:
            self.secrets.extend(str(row[0]) for row in database.execute('SELECT password_hash FROM users'))
            self.secrets.extend(str(row[0]) for row in database.execute('SELECT token_hash FROM sessions'))

    def no_secrets(self, value: str, label: str, kind: str = 'real-browser') -> None:
        """对未经持久脱敏器处理的内存内容执行已知秘密原值检查。"""
        self.check(all(not secret or secret not in value for secret in self.secrets), label, kind)

    def fill_change(self, page: Page, current: str, new: str, confirmation: str | None = None) -> None:
        """只向当前页面输入原值，不修改或截断密码。"""
        self.secrets.extend((current, new, new if confirmation is None else confirmation))
        page.locator('#account-current-password').fill(current)
        page.locator('#account-new-password').fill(new)
        page.locator('#account-confirm-password').fill(new if confirmation is None else confirmation)

    def validation(self, page: Page) -> None:
        """验证独立掩码、键盘切换、提示关联和前端无效输入阻断。"""
        self.stage = '密码输入与前端校验'
        section = self.settings(page)
        fields = [page.locator('#account-' + identity) for identity in ('current-password', 'new-password', 'confirm-password', 'revoke-password')]
        self.cleared(page, '账号四个密码框默认清空且独立掩码')
        self.check(page.locator('#account-new-password').get_attribute('aria-describedby') is not None and 'account-new-password-hint' in str(page.locator('#account-new-password').get_attribute('aria-describedby')), '新密码与中文复杂度提示关联')
        self.check(all(field.get_attribute('autocomplete') == mode for field, mode in zip(fields, ('current-password', 'new-password', 'new-password', 'current-password'))), '四个密码框使用各自适当的密码管理器语义')
        before = len(self.mutations)
        fields[0].fill(self.password)
        fields[0].focus()
        page.keyboard.press('Tab')
        self.check(fields[0].locator('..').get_by_role('button').evaluate('(node) => node === document.activeElement'), 'Tab可从密码框进入对应显示按钮')
        page.keyboard.press('Space')
        expect(fields[0]).to_have_attribute('type', 'text')
        self.check(all(field.get_attribute('type') == 'password' for field in fields[1:]), '显示原密码不改变其他三个字段的掩码')
        page.keyboard.press('Enter')
        expect(fields[0]).to_have_attribute('type', 'password')
        self.check(len(self.mutations) == before, 'Space与Enter切换密码显示不触发账号写请求')
        cases: tuple[tuple[str, str, str], ...] = (('weak', 'weak', '弱密码'), ('Strong1!中文Ab', 'Strong1!中文Ab', '中文新密码'), ('GoodNew12!Ab', 'Different12!Ab', '确认不一致'))
        for new, confirmation, label in cases:
            self.fill_change(page, self.password, new, confirmation)
            page.locator('#account-confirm-password').press('Enter')
            page.wait_for_timeout(100)
            self.check(len(self.mutations) == before, label + '在前端阻断且无POST')
        self.clear_fields(page)
        self.cleared(page, '手动清空字段同时恢复掩码')
        self.check(section.get_by_role('form', name='修改密码', exact=True).count() == 1 and section.get_by_role('form', name='退出其他会话', exact=True).count() == 1, '改密与再认证为独立且可访问的表单')

    def wrong_password(self, page: Page) -> None:
        """验证真实400不会被全局401处理误退出，且请求结束清空密码。"""
        self.stage = '错误原密码'
        empty_field = page.locator('#account-revoke-password')
        empty_field.locator('..').get_by_role('button').click()
        self.check(empty_field.input_value() == '' and empty_field.get_attribute('type') == 'text', '另一表单的空密码框可独立显示，作为请求完成掩码恢复的边界')
        wrong = 'Wrong-' + secrets.token_hex(12) + 'A1!'
        self.secrets.append(wrong)
        self.fill_change(page, wrong, 'NextPassword12!Ab')
        with page.expect_response(lambda response: response.url.endswith('/api/account/password') and response.request.method == 'POST') as pending:
            page.get_by_role('button', name='修改密码', exact=True).click()
        self.check(pending.value.status == 400, '真实错误原密码返回400而非401')
        expect(page.locator('section[aria-labelledby="account-security-title"]')).to_be_visible()
        self.cleared(page, '真实错误原密码保持登录且请求结束清空全部密码')
        labels: tuple[tuple[str, str], ...] = (('当前密码', 'current'), ('新密码', 'new'), ('确认新密码', 'confirm'), ('验证当前密码', 'revoke'))
        self.check(all(page.get_by_label(label, exact=True).get_attribute('id') == 'account-' + identity + '-password' for label, identity in labels) and 'account-new-password-hint' in str(page.locator('#account-new-password').get_attribute('aria-describedby')), '敏感操作重置后四字段标签和新密码提示仍精确关联')

    def dto_and_storage(self, contexts: list[BrowserContext]) -> None:
        """检查真实DTO、localStorage和sessionStorage的原始内存内容。"""
        self.stage = '秘密与DTO检查'
        for context in contexts:
            self.remember_cookies(context)
            self.secrets.extend(str(page.evaluate('navigator.userAgent')) for page in context.pages)
        self.remember_database_secrets()
        for index, context in enumerate(contexts):
            response = context.request.get(self.harness.client.base + '/api/account/sessions')
            self.check(response.status == 200, '真实浏览器' + str(index + 1) + '可读取会话DTO')
            data: dict[str, Any] = response.json()
            self.no_secrets(json.dumps(data, ensure_ascii=False), '真实会话DTO不含已知密码、bearer及摘要')
            self.check(set(data) == {'currentSession', 'items', 'nextCursor'} and data['currentSession']['current'] is True and all(item['current'] is False for item in data['items']), '真实DTO单独标记实际当前会话')
            for page in context.pages:
                values: str = page.evaluate('JSON.stringify({local:{...localStorage},session:{...sessionStorage},cookie:document.cookie})')
                self.no_secrets(values, '浏览器原始存储与可见Cookie不含已知秘密')

    def real_pagination(self, playwright: Playwright, page: Page) -> None:
        """以真实成功登录产生多页会话，验证手动继续且不重复当前项。"""
        self.stage = '真实会话分页'
        api = playwright.request.new_context(base_url=self.harness.client.base, extra_http_headers={'Origin': self.harness.client.base})
        try:
            for _ in range(22):
                response = api.post('/api/login', data={'username': self.username, 'password': self.password})
                self.check(response.status == 200, '分页夹具使用实际成功登录创建会话')
                cookies: dict[str, Any] = api.storage_state()
                for cookie in cookies['cookies']:
                    if cookie['name'] == 'nas_session':
                        self.remember_session_token(cookie['value'])
        finally:
            api.dispose()
        current_token: str = next(cookie['value'] for cookie in page.context.cookies() if cookie['name'] == 'nas_session')
        with closing(self.harness.database()) as database:
            expected_current: str = str(database.execute('SELECT session_id FROM sessions WHERE token_hash=?', (hashlib.sha256(current_token.encode()).hexdigest(),)).fetchone()[0])
            expected_other: set[str] = {str(row[0]) for row in database.execute('SELECT session_id FROM sessions WHERE session_id<>?', (expected_current,))}
        section = self.settings(page)
        with page.expect_response(lambda response: '/api/account/sessions' in response.url and response.request.method == 'GET'):
            section.get_by_role('button', name='刷新会话', exact=True).click()
        continuation = section.get_by_role('button', name='继续查看', exact=True)
        expect(continuation).to_be_visible()
        before: list[str] = section.locator('[data-current="false"]').evaluate_all('(nodes) => nodes.map(node => node.dataset.sessionId)')
        with page.expect_response(lambda response: '/api/account/sessions?' in response.url and 'cursor=' in response.url) as pending:
            continuation.click()
        expect(continuation).to_have_count(0)
        ids: list[str] = section.locator('[data-session-id]').evaluate_all('(nodes) => nodes.map(node => node.dataset.sessionId)')
        after: list[str] = section.locator('[data-current="false"]').evaluate_all('(nodes) => nodes.map(node => node.dataset.sessionId)')
        self.check(bool(before) and bool(after) and len(ids) == len(set(ids)) and not set(before).intersection(after), '真实多页会话手动切换有界批次且跨批编号无重复')
        self.check(pending.value.status == 200 and len(before) <= 20 and len(after) <= 20 and set(before + after) == expected_other, '真实两页结果与隔离数据库全部其他会话相等且每批有界')
        self.check(section.locator('[data-current="true"]').count() == 1 and section.locator('[data-current="true"]').get_attribute('data-session-id') == expected_current, '真实分页后当前会话仍固定为实际Cookie对应的唯一会话')

    def presentation(self, page: Page) -> None:
        """明确使用受控响应验证空页、服务错误、429和迟到响应，不计作后端证据。"""
        self.stage = '受控界面响应'
        section = self.settings(page)
        endpoint: str = self.harness.client.base + '/api/account/sessions'
        actual = page.request.get(endpoint).json()
        calls: list[str] = []

        def empty_batch(route: Route) -> None:
            """首批返回空数据和游标，下一批才结束。"""
            calls.append(route.request.url)
            route.fulfill(json={'currentSession': actual['currentSession'], 'items': [], 'nextCursor': None if 'cursor=' in route.request.url else 'v1.' + '0' * 32})

        page.route('**/api/account/sessions?*', empty_batch)
        try:
            section.get_by_role('button', name='刷新会话', exact=True).click()
            expect(section.get_by_text('本批无其他有效会话，可继续查看', exact=False)).to_be_visible()
            page.wait_for_timeout(150)
            self.check(len(calls) == 1, '受控UI：空批次有游标时保留继续按钮且不自动追页', 'presentation-fixture')
            section.get_by_role('button', name='继续查看', exact=True).click()
            expect(section.get_by_role('button', name='继续查看', exact=True)).to_have_count(0)
            self.check(len(calls) == 2, '受控UI：空批次可手动继续到结束', 'presentation-fixture')
        finally:
            page.unroute('**/api/account/sessions?*', empty_batch)

        def unavailable(route: Route) -> None:
            """仅给当前会话读取注入明确的503。"""
            route.fulfill(status=503, json={'error': '受控会话读取失败'})

        page.route('**/api/account/sessions?*', unavailable)
        try:
            section.get_by_role('button', name='刷新会话', exact=True).click()
            expect(section.get_by_text('受控会话读取失败', exact=False)).to_be_visible()
            self.check(True, '受控UI：读取失败显示独立错误而非成功空态', 'presentation-fixture')
        finally:
            page.unroute('**/api/account/sessions?*', unavailable)
        section.get_by_role('button', name='刷新会话', exact=True).click()
        expect(section.locator('[data-current="true"]')).to_be_visible()

        pending_write: list[Route] = []

        def limited(route: Route) -> None:
            """暂存账号改密，待验证全局敏感写互斥后返回429。"""
            pending_write.append(route)

        page.route('**/api/account/password', limited)
        try:
            self.fill_change(page, self.password, 'Presentation12!Ab')
            before = len(self.mutations)
            page.get_by_role('button', name='修改密码', exact=True).click()
            page.wait_for_timeout(100)
            self.check(len(pending_write) == 1, '受控UI：一次提交只创建一个敏感请求', 'presentation-fixture')
            self.check(all(field.is_disabled() for field in section.locator('input').all()) and section.get_by_role('button', name='退出其他会话', exact=True).is_disabled() and section.get_by_role('button', name='撤销当前会话', exact=True).is_disabled(), '受控UI：敏感写进行中同时禁用其他密码与撤销操作', 'presentation-fixture')
            pending_write[0].fulfill(status=429, headers={'Retry-After': '17'}, json={'error': '受控再认证次数过多'})
            expect(section.get_by_text('受控再认证次数过多', exact=False)).to_be_visible()
            self.check('17' in section.locator('#account-password-error').inner_text(), '受控UI：429展示Retry-After建议等待秒数', 'presentation-fixture')
            self.cleared(page, '受控UI：429结束后清空密码并恢复掩码', 'presentation-fixture')
            page.wait_for_timeout(200)
            self.check(len(self.mutations) == before + 1, '受控UI：限流不会自动重发敏感写请求', 'presentation-fixture')
        finally:
            page.unroute('**/api/account/password', limited)

        reads: list[str] = []

        def read_after_conflict(route: Route) -> None:
            """冲突后仅记录实际只读刷新，不拦截其真实响应。"""
            reads.append('GET')
            route.continue_()

        def conflict(route: Route) -> None:
            """模拟凭据快照冲突，不修改真实账号。"""
            route.fulfill(status=409, json={'error': '受控账号已变化，请重新确认'})

        page.route('**/api/account/password', conflict)
        page.route('**/api/account/sessions?*', read_after_conflict)
        try:
            self.fill_change(page, self.password, 'Presentation12!Ab')
            before = len(self.mutations)
            page.get_by_role('button', name='修改密码', exact=True).click()
            expect(section.get_by_text('受控账号已变化，请重新确认', exact=False)).to_be_visible()
            self.cleared(page, '受控UI：409清空密码并保持登录', 'presentation-fixture')
            page.wait_for_timeout(150)
            self.check(len(reads) == 1 and len(self.mutations) == before + 1, '受控UI：409只刷新一次只读状态，不重发敏感写', 'presentation-fixture')
        finally:
            page.unroute('**/api/account/password', conflict)
            page.unroute('**/api/account/sessions?*', read_after_conflict)

        def uncertain(route: Route) -> None:
            """制造网络结果未知的界面条件，不把它称为后端执行。"""
            route.abort('failed')

        page.route('**/api/account/password', uncertain)
        try:
            self.fill_change(page, self.password, 'Presentation12!Ab')
            before = len(self.mutations)
            page.get_by_role('button', name='修改密码', exact=True).click()
            expect(section.get_by_text('操作结果尚未确认', exact=False)).to_be_visible()
            self.cleared(page, '受控UI：网络结果未知时清空密码', 'presentation-fixture')
            page.wait_for_timeout(200)
            self.check(len(self.mutations) == before + 1, '受控UI：网络结果未知提示先核对，且不自动重试', 'presentation-fixture')
        finally:
            page.unroute('**/api/account/password', uncertain)

        pending: list[Route] = []

        def delayed(route: Route) -> None:
            """暂存读响应直到组件已卸载，不在回调里阻塞浏览器事件。"""
            pending.append(route)

        page.route('**/api/account/sessions?*', delayed)
        try:
            section.get_by_role('button', name='刷新会话', exact=True).click()
            page.wait_for_timeout(100)
            self.check(len(pending) == 1, '受控UI：已捕获待返回的会话读取', 'presentation-fixture')
            page.locator('#account-current-password').fill(self.password)
            page.evaluate("location.hash = '#home'")
            expect(section).to_have_count(0)
            try:
                pending[0].fulfill(json=actual)
            except PlaywrightError:
                # 组件正确中止fetch后，浏览器允许直接拒绝迟到路由的fulfill。
                pass
        finally:
            page.unroute('**/api/account/sessions?*', delayed)
        self.settings(page)
        self.cleared(page, '受控UI：离开设置后迟到读响应不恢复秘密，重新进入保持清空', 'presentation-fixture')

    def layout(self, page: Page) -> None:
        """在四种宽度和明暗主题检查账号区域及当前撤销对话框。"""
        self.stage = '响应式与键盘对话框'
        self.clear_fields(page)
        section = self.settings(page)
        for width in (375, 768, 1024, 1440):
            page.set_viewport_size({'width': width, 'height': 960})
            for theme in ('light', 'dark'):
                if page.locator('html').get_attribute('data-theme') != theme:
                    page.get_by_role('button', name=re.compile('切换到.*主题')).first.click()
                section.scroll_into_view_if_needed()
                dimensions: dict[str, Any] = page.evaluate('''() => ({viewport:innerWidth,document:document.documentElement.scrollWidth,fields:[...document.querySelectorAll('section[aria-labelledby="account-security-title"] input,section[aria-labelledby="account-security-title"] button')].filter(node=>node.getClientRects().length).map(node=>({width:node.getBoundingClientRect().width,height:node.getBoundingClientRect().height,left:node.getBoundingClientRect().left,right:node.getBoundingClientRect().right}))})''')
                self.check(dimensions['document'] <= width + 1 and all(item['left'] >= -1 and item['right'] <= width + 1 for item in dimensions['fields']), str(width) + 'px ' + theme + '账号页无横向溢出')
                self.check(all(item['width'] >= 44 and item['height'] >= 44 for item in dimensions['fields']), str(width) + 'px ' + theme + '账号输入与按钮触控尺寸至少44px')
                self.cleared(page, '截图前敏感字段清空且掩码')
                name = 'account-' + str(width) + '-' + theme + '.png'
                page.screenshot(path=str(self.root / name), full_page=True)
                self.screenshots.append(name)
                trigger = section.get_by_role('button', name='撤销当前会话', exact=True)
                trigger.click()
                dialog = page.get_by_role('dialog')
                expect(dialog.get_by_role('heading', name='撤销当前会话', exact=True)).to_be_visible()
                dimensions = dialog.evaluate('(node) => ({width:node.clientWidth,scroll:node.scrollWidth,left:node.getBoundingClientRect().left,right:node.getBoundingClientRect().right})')
                self.check(dimensions['scroll'] <= dimensions['width'] + 1 and dimensions['left'] >= -1 and dimensions['right'] <= width + 1, str(width) + 'px ' + theme + '当前撤销确认框无横向溢出')
                focus_safe: bool = True
                for key in ('Tab', 'Tab', 'Tab', 'Tab', 'Shift+Tab', 'Shift+Tab', 'Shift+Tab', 'Shift+Tab'):
                    page.keyboard.press(key)
                    # 原生模态允许Tab进入浏览器工具栏；页面仍有焦点时必须留在对话框。
                    focus_safe = focus_safe and dialog.evaluate('(node) => node.contains(document.activeElement) || (!document.hasFocus() && document.activeElement === document.body)')
                self.check(focus_safe, str(width) + 'px ' + theme + '撤销确认框正反Tab不把网页焦点移入模态背景')
                page.locator('#account-current-password').evaluate('(node) => node.focus()')
                self.check(dialog.evaluate('(node) => node.contains(document.activeElement)'), str(width) + 'px ' + theme + '原生模态阻止背景输入框获取焦点')
                if (width, theme) in ((375, 'light'), (1440, 'dark')):
                    name = 'account-dialog-' + str(width) + '-' + theme + '.png'
                    dialog.screenshot(path=str(self.root / name))
                    self.screenshots.append(name)
                page.keyboard.press('Escape')
                expect(dialog).not_to_be_visible()
                self.check(trigger.evaluate('(node) => document.activeElement === node'), str(width) + 'px ' + theme + 'Escape关闭后焦点返回当前撤销按钮')

    def session_mutations(self, first: Page, second: Page, first_context: BrowserContext, second_context: BrowserContext) -> None:
        """通过实际界面撤销其他、退出其他、改密和撤销当前会话。"""
        self.stage = '真实会话撤销'
        section = self.settings(first)
        second_id: str = second.request.get(self.harness.client.base + '/api/account/sessions').json()['currentSession']['id']
        section.get_by_role('button', name='刷新会话', exact=True).click()
        expect(section.locator('[aria-busy="true"]')).to_have_count(0)
        for _ in range(3):
            if section.locator('[data-session-id="' + second_id + '"]').count():
                break
            continuation = section.get_by_role('button', name='继续查看', exact=True)
            expect(continuation).to_be_visible()
            with first.expect_response(lambda response: '/api/account/sessions?' in response.url and 'cursor=' in response.url):
                continuation.click()
            expect(section.locator('[aria-busy="true"]')).to_have_count(0)
        with first.expect_response(lambda response: response.url.endswith('/api/account/sessions/' + second_id) and response.request.method == 'DELETE') as pending:
            section.locator('[data-session-id="' + second_id + '"]').get_by_role('button', name=re.compile('撤销会话')).click()
        self.check(pending.value.status == 200 and second.request.get(self.harness.client.base + '/api/stats').status == 401, '真实界面撤销指定其他会话，其旧Cookie立即失效')
        self.login(second, self.password)
        self.remember_cookies(second_context)
        first.locator('#account-revoke-password').fill(self.password)
        with first.expect_response(lambda response: response.url.endswith('/api/account/sessions/revoke-others')) as pending:
            section.get_by_role('button', name='退出其他会话', exact=True).click()
        data = pending.value.json()
        self.check(pending.value.status == 200 and data['requiresLogin'] is False and data['revokedCount'] >= 22, '真实再认证退出其他会话返回实际删除记录数')
        self.check(first.request.get(self.harness.client.base + '/api/stats').status == 200 and second.request.get(self.harness.client.base + '/api/stats').status == 401, '退出其他会话保留当前Cookie并使其他Cookie失效')
        self.cleared(first, '真实退出其他会话请求完成清空所有密码')
        self.login(second, self.password)
        self.remember_cookies(second_context)
        self.stage = '真实改密并重新登录'
        replacement: str = 'A1!' + secrets.token_hex(18) + 'z'
        self.secrets.append(replacement)
        self.fill_change(first, self.password, replacement)
        with first.expect_response(lambda response: response.url.endswith('/api/account/password') and response.request.method == 'POST') as pending:
            section.get_by_role('button', name='修改密码', exact=True).click()
        self.check(pending.value.status == 200 and pending.value.json() == {'requiresLogin': True}, '真实改密成功明确要求重新登录')
        expect(first.get_by_role('button', name='进入内容空间', exact=True)).to_be_visible()
        self.check(first.request.get(self.harness.client.base + '/api/stats').status == 401 and second.request.get(self.harness.client.base + '/api/stats').status == 401, '真实改密使当前及其他所有旧Cookie失效')
        response = second.request.post(self.harness.client.base + '/api/login', data={'username': self.username, 'password': self.password}, headers={'Origin': self.harness.client.base})
        self.check(response.status == 401, '真实改密后原密码不能重新登录')
        self.login(first, replacement)
        self.remember_cookies(first_context)
        self.check(True, '真实新密码原值可以重新登录')
        self.settings(first)
        self.cleared(first, '重新登录后账号表单没有遗留秘密')
        self.stage = '真实撤销当前会话'
        first.get_by_role('button', name='撤销当前会话', exact=True).click()
        dialog = first.get_by_role('dialog')
        with first.expect_response(lambda response: '/api/account/sessions/' in response.url and response.request.method == 'DELETE') as pending:
            dialog.get_by_role('button', name='确认退出当前会话', exact=True).click()
        self.check(pending.value.status == 200 and pending.value.json()['requiresLogin'] is True, '真实撤销当前会话返回重新登录标记')
        expect(first.get_by_role('button', name='进入内容空间', exact=True)).to_be_visible()
        self.check(first.request.get(self.harness.client.base + '/api/stats').status == 401 and not any(cookie['name'] == 'nas_session' for cookie in first_context.cookies()), '真实撤销当前会话后Cookie清除且受保护读取401')

    def run(self, playwright: Playwright, browser: Browser) -> None:
        """完成真实浏览器场景，并在所有路径回收两个隔离上下文。"""
        contexts: list[BrowserContext] = []
        try:
            for _ in range(2):
                contexts.append(browser.new_context(viewport={'width': 1440, 'height': 960}, reduced_motion='reduce'))
            pages: list[Page] = [context.new_page() for context in contexts]
            for page in pages:
                self.observe(page)
                self.login(page, self.password)
            current_ids: list[str] = [page.request.get(self.harness.client.base + '/api/account/sessions').json()['currentSession']['id'] for page in pages]
            self.check(len(set(current_ids)) == 2, '两个独立浏览器上下文通过真实登录创建不同会话')
            self.validation(pages[0])
            self.real_pagination(playwright, pages[0])
            self.wrong_password(pages[0])
            self.dto_and_storage(contexts)
            self.presentation(pages[0])
            self.layout(pages[0])
            self.session_mutations(pages[0], pages[1], contexts[0], contexts[1])
            self.stage = '最终浏览器秘密检查'
            self.remember_database_secrets()
            self.check(self.page_error_count == 0, '两个真实页面无未处理脚本异常')
            self.check(not self.console_overflow, '原始控制台检查未超过内存采样预算')
            self.no_secrets('\n'.join(self.console), '内存原始浏览器控制台不含已知密码、bearer及摘要')
            for context in contexts:
                for page in context.pages:
                    self.no_secrets(page.evaluate('JSON.stringify({local:{...localStorage},session:{...sessionStorage}})'), '最终浏览器存储不含已知秘密')
            self.stage = '已完成'
        finally:
            for context in reversed(contexts):
                try:
                    context.close()
                except BaseException:
                    # 继续回收其余自有上下文，任何关闭失败仍使整体验收失败。
                    self.cleanup_failed = True
            if self.cleanup_failed:
                raise RuntimeError('浏览器上下文清理失败')


def main() -> int:
    """校验本地产物和浏览器路径，在项目隔离环境执行并持久化安全结果。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, help='已构建主程序路径，仅运行自有回环实例')
    parser.add_argument('--web-dir', default='web/dist', help='对应本次代码的已构建前端目录')
    parser.add_argument('--browser', default='C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe', help='本地Chromium兼容浏览器可执行文件，不自动安装')
    parser.add_argument('--root', help='仓库.runtime/verify内尚不存在的本次证据目录')
    args = parser.parse_args()
    binary: Path = Path(args.binary).resolve()
    web: Path = Path(args.web_dir).resolve()
    executable: Path = Path(args.browser).resolve()
    if not binary.is_file() or not executable.is_file() or not (web / 'index.html').is_file():
        parser.error('主程序、浏览器或已构建前端缺失，不能记为通过')
    try:
        root = new_run_root(args.root, 'account-browser')
    except Blocked as error:
        parser.error(str(error))
    # 禁止继承Playwright调试开关，以免驱动把密码输入输出为原始调试日志。
    os.environ.pop('DEBUG', None)
    os.environ.pop('PWDEBUG', None)
    password: str = 'A1!' + secrets.token_hex(18) + 'z'
    harness = AccountHarness(binary, root / 'application', [])
    checks = AccountBrowserChecks(harness, root, password)
    artifact: dict[str, Any] = file_identity(binary)
    frontend: dict[str, Any] | None = None
    frontend_unchanged: bool = False
    browser_version: str | None = None
    browser: Browser | None = None
    passed: bool = False
    failure_kind: str | None = None
    cleanup_failed: bool = False
    try:
        source_frontend: dict[str, Any] = frontend_identity(web)
        shutil.copytree(web, harness.root / 'web', dirs_exist_ok=True)
        frontend = frontend_identity(harness.root / 'web')
        checks.check(frontend == source_frontend, '隔离实例前端首页和全部构建资产与指定输入摘要一致', 'artifact-identity')
        harness.start()
        code, _, _ = harness.client.request('/api/setup', 'POST', {'username': checks.username, 'password': password})
        checks.check(code == 201, '隔离真实后端创建专用浏览器验证账号')
        for cookie in harness.client.cookies:
            if cookie.name == 'nas_session':
                checks.remember_session_token(cookie.value)
        checks.remember_database_secrets()
        with sync_playwright() as playwright:
            try:
                browser = playwright.chromium.launch(executable_path=str(executable), headless=True)
                browser_version = browser.version
                checks.run(playwright, browser)
                frontend_unchanged = frontend_identity(harness.root / 'web') == frontend
                checks.check(frontend_unchanged, '浏览器验收前后实际服务前端首页和构建资产摘要未变', 'artifact-identity')
                passed = True
            finally:
                if browser is not None:
                    try:
                        browser.close()
                    except BaseException:
                        # 保留业务失败并继续回收隔离应用，不能让浏览器清理失败成为通过。
                        cleanup_failed = True
                        passed = False
    except BaseException as error:
        # Playwright异常可包含输入值或DOM；持久记录和终端均只保留异常类型及固定阶段。
        failure_kind = type(error).__name__
        passed = False
    finally:
        cleanup_failed = cleanup_failed or checks.cleanup_failed
        if cleanup_failed:
            passed = False
        try:
            harness.stop()
        except BaseException:
            cleanup_failed = True
            passed = False
        write_json(root / 'assertions.json', checks.assertions)
        write_json(root / 'result.json', {'passed': passed, 'assertions': len(checks.assertions), 'realBrowserAssertions': sum(item['kind'] == 'real-browser' for item in checks.assertions), 'presentationAssertions': sum(item['kind'] == 'presentation-fixture' for item in checks.assertions), 'artifactAssertions': sum(item['kind'] == 'artifact-identity' for item in checks.assertions), 'stage': checks.stage, 'failureKind': failure_kind, 'cleanupFailed': cleanup_failed, 'artifact': artifact, 'frontend': frontend, 'frontendUnchanged': frontend_unchanged, 'browserVersion': browser_version, 'screenshots': checks.screenshots, 'realApplication': True, 'realExternalInstances': False, 'targetNAS': False, 'limits': ['真实Windows浏览器结果不代表Linux或NAS实机验收', '受控route仅证明界面呈现，不计作真实后端执行', '秘密检查覆盖原始内存DTO、浏览器控制台及存储，不宣称扫描全部服务端原始stdout', '保存的服务端日志由公用夹具脱敏，不作为原始日志无秘密的动态证明']})
    print(('账号浏览器验证通过' if passed else '账号浏览器验证未完成') + '：' + str(len(checks.assertions)) + ' 项；阶段：' + checks.stage + ('；清理失败' if cleanup_failed else ''), flush=True)
    return 0 if passed else 1


if __name__ == '__main__':
    from project_env import run_cli

    raise SystemExit(run_cli(main))
