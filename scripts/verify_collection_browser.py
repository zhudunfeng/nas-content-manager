"""对verify_collection --serve隔离实例执行真实浏览器验收；需本机Playwright和浏览器。"""
from __future__ import annotations

import sys
if __name__ == '__main__':
    sys.dont_write_bytecode = True

import argparse
import json
from pathlib import Path
import re
import shutil
import uuid
from typing import Any

from playwright.sync_api import Page, Route, expect, sync_playwright


class BrowserChecks:
    """记录真实页面操作以及明确标注的受控错误响应。"""

    def __init__(self, page: Page, root: Path) -> None:
        """绑定本次页面和证据目录。"""
        self.page: Page = page  # 隔离浏览器页面。
        self.root: Path = root  # 本次验证输出目录。
        self.passed: int = 0  # 实际通过的断言数量。
        self.page_errors: list[str] = []  # 未处理脚本异常。
        self.console_errors: list[str] = []  # 包含主动注入HTTP错误的浏览器日志。
        page.on('pageerror', lambda error: self.page_errors.append(str(error)))
        page.on('console', lambda message: self.console_errors.append(message.text) if message.type == 'error' else None)

    def check(self, condition: bool, message: str) -> None:
        """断言实际状态并输出中文证据。"""
        if not condition:
            raise AssertionError(message)
        self.passed += 1
        print('  ✓ ' + message, flush=True)

    def tab(self, name: str) -> None:
        """点击收集中心当前可见分区。"""
        self.page.locator('.collection-tabs').get_by_role('button', name=name, exact=True).click()

    def no_overflow(self, label: str) -> None:
        """检查文档和打开的模态框边界，保留允许滚动的内部表格。"""
        dimensions = self.page.evaluate('''() => ({width:innerWidth,document:document.documentElement.scrollWidth,dialogs:[...document.querySelectorAll('dialog[open]')].map(d=>({width:d.clientWidth,scroll:d.scrollWidth,left:d.getBoundingClientRect().left,right:d.getBoundingClientRect().right}))})''')
        self.check(dimensions['document'] <= dimensions['width'] + 1 and all(item['scroll'] <= item['width'] + 1 and item['left'] >= -1 and item['right'] <= dimensions['width'] + 1 for item in dimensions['dialogs']), label + '无文档或弹窗横向溢出')

    def run(self, credentials: dict[str, str], base: str) -> None:
        """验证真实来源、预览、任务、跳转及响应式交互。"""
        page = self.page
        page.goto(base)
        page.get_by_label('用户名', exact=True).fill(credentials['username'])
        page.get_by_label('密码', exact=True).fill(credentials['password'])
        page.get_by_role('button', name='进入内容空间', exact=True).click()
        page.get_by_role('button', name='资源收集', exact=True).click()
        expect(page.get_by_role('heading', name='资源收集', exact=True)).to_be_visible()
        expect(page.get_by_text('获取执行端已连接。', exact=False)).to_be_visible()
        self.check(True, '真实账户登录并显示执行端能力')
        self.tab('订阅')
        if page.get_by_text('还没有订阅规则', exact=False).count():
            expect(page.get_by_text('还没有订阅规则', exact=False)).to_be_visible()
            self.check(True, '成功空订阅显示空状态')
        self.subscription()
        self.tab('来源')
        suffix = uuid.uuid4().hex[:8]
        created_name = '页面创建音乐-' + suffix
        edited_name = '页面修订音乐-' + suffix
        source_directory = self.root / 'media' / ('browser-' + suffix)
        source_directory.mkdir()
        shutil.copyfile(self.root / 'media/music/收集样本.wav', source_directory / '收集样本.wav')
        page.get_by_role('button', name='添加来源', exact=True).click()
        dialog = page.get_by_role('dialog')
        expect(dialog).to_be_visible()
        page.keyboard.press('Tab')
        self.check(dialog.evaluate('(d)=>d.contains(document.activeElement)'), '来源弹窗键盘焦点留在模态框内')
        page.keyboard.press('Escape')
        expect(dialog).not_to_be_visible()
        self.check(page.get_by_role('button', name='添加来源', exact=True).evaluate('(n)=>document.activeElement===n'), 'Escape关闭后焦点回到来源按钮')
        page.get_by_role('button', name='添加来源', exact=True).click()
        dialog = page.get_by_role('dialog')
        dialog.get_by_label('来源类型', exact=False).select_option('local')
        dialog.get_by_label('来源名称', exact=False).fill(created_name)
        dialog.get_by_label('已授权媒体根目录', exact=False).select_option('media-0')
        dialog.get_by_label('来源子目录', exact=False).fill(source_directory.name)
        dialog.get_by_role('button', name='保存来源', exact=True).click()
        expect(page.get_by_role('heading', name=created_name, exact=True)).to_be_visible()
        record = page.locator('article').filter(has=page.get_by_role('heading', name=created_name, exact=True))
        record.get_by_role('button', name='编辑', exact=True).click()
        page.get_by_role('dialog').get_by_label('来源名称', exact=False).fill(edited_name)
        page.get_by_role('button', name='保存来源', exact=True).click()
        expect(page.get_by_role('heading', name=edited_name, exact=True)).to_be_visible()
        self.check(True, '真实API创建并修订本地来源')
        record = page.locator('article').filter(has=page.get_by_role('heading', name=edited_name, exact=True))
        record.get_by_role('button', name='预览导入', exact=True).click()
        dialog = page.get_by_role('dialog')
        dialog.get_by_role('button', name='开始预览', exact=True).click()
        expect(dialog.get_by_text('收集样本.wav', exact=True)).to_be_visible()
        dialog.get_by_label('内容类型', exact=False).select_option('music')
        # 先选旧库再切换建库，检查隐藏目标不会残留在请求中。
        dialog.get_by_label('创建独立媒体库', exact=False).uncheck()
        library = dialog.get_by_label('已有媒体库', exact=False)
        if library.count():
            first_library = library.locator('option').nth(1).get_attribute('value')
            library.select_option(first_library)
        dialog.get_by_label('创建独立媒体库', exact=False).check()
        calls: list[dict[str, Any]] = []
        accepted_ids: list[str] = []
        def submit_response(route: Route) -> None:
            """首次真实受理后丢弃响应，再让浏览器用原请求恢复。"""
            calls.append(route.request.post_data_json)
            if len(calls) == 1:
                response = route.fetch()
                accepted_ids.append(response.json()['task']['id'])
                route.abort('failed')
            else:
                route.continue_()
        page.route('**/api/collection/tasks', submit_response)
        try:
            dialog.get_by_role('button', name=re.compile('确认收集')).click()
            expect(dialog.get_by_role('button', name='重试未成功条目', exact=True)).to_be_enabled()
            dialog.get_by_role('button', name='重试未成功条目', exact=True).click()
            expect(dialog.get_by_role('button', name='查看收集任务', exact=False)).to_be_visible()
            self.check(len(calls) == 2 and calls[0] == calls[1] and bool(accepted_ids), '真实受理响应丢失后前端保留完整计划和原幂等键')
            self.check(calls[0]['createLibrary'] and calls[0]['libraryId'] == 0, '切换独立建库后不提交隐藏的旧媒体库编号')
        finally:
            page.unroute('**/api/collection/tasks', submit_response)
        expect(dialog.get_by_role('button', name='查看收集任务', exact=False)).to_be_visible()
        dialog.get_by_role('button', name='查看收集任务', exact=False).click()
        expect(page.get_by_role('button', name='打开媒体 1', exact=False)).to_be_visible(timeout=30000)
        self.check(page.locator('dialog[open]').count() == 1, '真实本地预览、任务受理与扫描入库完成且仅一个弹窗')
        page.get_by_role('button', name='打开媒体 1', exact=False).click()
        page.get_by_role('button', name='开始播放', exact=True).click()
        expect(page.locator('audio')).to_have_count(1)
        self.check(True, '收集任务可以打开真实音乐媒体')
        page.keyboard.press('Escape')
        page.get_by_role('button', name='资源收集', exact=True).click()
        self.tab('候选')
        acquire = page.get_by_role('button', name='确认获取', exact=True)
        if acquire.count():
            acquire.click()
            dialog = page.get_by_role('dialog')
            dialog.get_by_label('内容类型', exact=False).select_option('novel')
            dialog.get_by_label('创建独立媒体库', exact=False).check()
            dialog.get_by_role('button', name='确认获取', exact=True).click()
        else:
            page.get_by_role('button', name='查看关联任务', exact=True).click()
        expect(page.get_by_role('button', name='前往下载详情与选文件', exact=True)).to_be_visible(timeout=30000)
        page.get_by_role('button', name='前往下载详情与选文件', exact=True).click()
        expect(page.get_by_role('checkbox', name=re.compile('章节.txt'))).to_be_visible(timeout=30000)
        self.check(page.locator('dialog[open]').count() == 1, 'RSS候选任务跳转原BT文件选择详情')
        page.keyboard.press('Escape')
        page.get_by_role('button', name='资源收集', exact=True).click()
        self.partial_batch()
        self.query_error()
        self.tab('任务')
        page.get_by_label('获取阶段', exact=False).select_option('canceled')
        expect(page.get_by_text('此阶段没有任务', exact=True)).to_be_visible()
        self.check(True, '真实任务筛选空结果不显示网络错误')
        page.get_by_label('获取阶段', exact=False).select_option('')
        for width in (375, 768, 1024, 1440):
            page.set_viewport_size({'width': width, 'height': 960})
            for theme in ('light', 'dark'):
                if page.locator('html').get_attribute('data-theme') != theme:
                    page.get_by_role('button', name=re.compile('切换到.*主题')).click()
                self.tab('概览')
                self.no_overflow(f'{width}px {theme}概览')
                if width == 375:
                    self.check(page.locator('.bottom-nav button:visible').count() == 5, '手机底栏保持五项')
                    page.get_by_role('button', name='更多内容与管理', exact=True).click()
                    page.get_by_role('button', name='资源收集', exact=True).last.click()
                self.tab('来源')
                page.get_by_role('button', name='添加来源', exact=True).click()
                self.no_overflow(f'{width}px {theme}来源弹窗')
                if width in (375, 1440):
                    page.screenshot(path=str(self.root / f'collection-{width}-{theme}.png'), full_page=True)
                page.keyboard.press('Escape')
        self.check(not self.page_errors, '页面无未处理脚本异常')

    def partial_batch(self) -> None:
        """仅注入预览响应检查批次部分失败呈现，不将其计为真实网页获取。"""
        page = self.page
        calls: list[str] = []
        def response(route: Route) -> None:
            """分别返回受控错误和只用于展示的有限预览。"""
            body: dict[str, Any] = route.request.post_data_json
            calls.append(body['url'])
            if len(calls) == 1:
                route.fulfill(status=422, json={'error': '受控网页预览失败'})
            else:
                route.fulfill(json={'preview': {'id': 'display-only', 'sourceId': body['sourceId'], 'title': '受控成功预览', 'kind': 'web', 'profile': 'video', 'files': [], 'formats': [], 'totalBytes': 1024, 'expiresAt': '2099-01-01T00:00:00Z', 'warnings': []}})
        page.route('**/api/collection/previews', response)
        try:
            page.get_by_role('button', name='网页与文件', exact=True).click()
            dialog = page.get_by_role('dialog')
            options = dialog.get_by_label('收集来源', exact=False).locator('option')
            target = next(option.get_attribute('value') for option in options.all() if '浏览器网页入口' in option.inner_text())
            dialog.get_by_label('收集来源', exact=False).select_option(target)
            dialog.get_by_label('页面链接', exact=False).fill('https://example.com/fail\nhttps://example.com/success')
            dialog.get_by_role('button', name='开始预览', exact=True).click()
            expect(dialog.get_by_text('受控网页预览失败', exact=True)).to_be_visible()
            expect(dialog.get_by_text('受控成功预览', exact=True)).to_be_visible()
            self.check(len(calls) == 2, '受控UI响应：批次失败保留且继续下一项')
            self.no_overflow('批次部分失败弹窗')
            page.keyboard.press('Escape')
        finally:
            page.unroute('**/api/collection/previews', response)

    def subscription(self) -> None:
        """保存默认暂停人工规则并试运行，验证自动授权不预勾。"""
        page = self.page
        endpoint: str = page.evaluate('location.origin') + '/api/collection/tasks'
        before = page.request.get(endpoint).json()['total']
        page.get_by_role('button', name='添加订阅', exact=True).first.click()
        dialog = page.get_by_role('dialog')
        name = '页面订阅-' + uuid.uuid4().hex[:8]
        dialog.get_by_label('规则名称', exact=False).fill(name)
        sources = dialog.get_by_label('RSS 来源', exact=False)
        sources.select_option(sources.locator('option').nth(1).get_attribute('value'))
        self.check(not dialog.get_by_label('启用定时订阅', exact=False).is_checked(), '新订阅默认暂停')
        dialog.get_by_label('执行方式', exact=False).select_option('automatic')
        self.check(not dialog.get_by_label('我授权此规则', exact=False).is_checked() and dialog.get_by_role('button', name='保存规则', exact=True).is_disabled(), '自动获取必须重新主动授权')
        dialog.get_by_label('执行方式', exact=False).select_option('review')
        dialog.get_by_label('包含关键词', exact=False).fill('浏览器')
        dialog.get_by_label('内容类型', exact=False).select_option('novel')
        dialog.get_by_role('button', name='保存规则', exact=True).click()
        expect(page.get_by_role('heading', name=name, exact=True)).to_be_visible()
        record = page.locator('article').filter(has=page.get_by_role('heading', name=name, exact=True))
        record.get_by_role('button', name='试运行', exact=True).click()
        expect(page.get_by_role('dialog').get_by_text(re.compile('此次试运行没有创建获取任务'))).to_be_visible()
        self.check(page.request.get(endpoint).json()['total'] == before, '真实订阅保存及规则试运行不产生获取任务')
        page.keyboard.press('Escape')

    def query_error(self) -> None:
        """注入单个列表失败并确认没有伪装成成功空列表。"""
        page = self.page
        def response(route: Route) -> None:
            """只替换来源列表响应，其他真实接口继续工作。"""
            route.fulfill(status=503, json={'error': '受控来源列表离线'})
        page.route('**/api/collection/sources', response)
        try:
            self.tab('来源')
            page.get_by_role('button', name='刷新收集状态', exact=True).click()
            expect(page.get_by_text(re.compile('受控来源列表离线'))).to_be_visible()
            self.check(True, '受控UI响应：来源查询失败显示独立错误')
        finally:
            page.unroute('**/api/collection/sources', response)
        page.get_by_role('button', name='刷新收集状态', exact=True).click()
        expect(page.get_by_text(re.compile('受控来源列表离线'))).not_to_be_visible()


def main() -> None:
    """解析仅供隔离服务的路径参数并保证浏览器释放。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, required=True, help='verify_collection --serve产生且包含verify路径段的目录')
    parser.add_argument('--browser', type=Path, required=True, help='已安装Chromium或Edge可执行文件的固定路径')
    args = parser.parse_args()
    root = args.root.resolve()
    if 'verify' not in root.parts or not args.browser.is_file():
        parser.error('仅允许隔离verify目录和已有浏览器')
    info = json.loads((root / 'browser.json').read_text(encoding='utf-8'))
    credentials = json.loads(Path(info['credentialFile']).read_text(encoding='utf-8'))
    if not info['base'].startswith('http://127.0.0.1:'):
        parser.error('只允许本机隔离应用')
    with sync_playwright() as runner:
        browser = runner.chromium.launch(executable_path=str(args.browser.resolve()), headless=True)
        checks = BrowserChecks(browser.new_page(viewport={'width': 1440, 'height': 960}), root)
        passed = False
        try:
            checks.run(credentials, info['base'])
            passed = True
        finally:
            checks.page.screenshot(path=str(root / 'browser-final.png'), full_page=True)
            browser.close()
            (root / 'browser-result.json').write_text(json.dumps({'passed': passed, 'assertions': checks.passed, 'pageErrors': checks.page_errors, 'consoleErrors': checks.console_errors, 'realWebAcquisition': False}, ensure_ascii=False, indent=2), encoding='utf-8')


if __name__ == '__main__':
    from project_env import run_cli
    raise SystemExit(run_cli(main))
