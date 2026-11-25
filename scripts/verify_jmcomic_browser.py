"""对隔离真实应用执行漫画入口与阅读浏览器验收，不访问真实漫画站点。"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
import time
from typing import Any
from urllib.parse import urlsplit

from playwright.sync_api import Browser, Page, Route, expect, sync_playwright

from project_env import ROOT, run_cli


class BrowserCheck:
    """保留独立断言与截图，区分真实接口链路和可控响应专项。"""

    def __init__(self, page: Page, output: Path) -> None:
        """绑定本次页面及证据目录，不记录登录凭据。"""
        self.page: Page = page  # 本次自有页面。
        self.output: Path = output  # 仓库隔离证据目录。
        self.assertions: list[str] = []  # 实际通过的中文检查点。

    def check(self, value: bool, description: str) -> None:
        """失败立即中止，避免把未执行项目记录为通过。"""
        if not value:
            raise AssertionError(description)
        self.assertions.append(description)

    def open_search(self) -> None:
        """从真实资源收集工具栏进入漫画弹窗。"""
        self.page.get_by_role('button', name='漫画搜索下载', exact=True).click()
        expect(self.page.get_by_role('dialog', name='JMComic 漫画搜索与下载')).to_be_visible()
        expect(self.page.locator('dialog[open]')).to_have_count(1)

    def source_switch(self) -> None:
        """经正式表单切换来源类型，验证隐藏配置不会泄漏到固定漫画来源。"""
        self.page.get_by_role('navigation', name='收集分区').get_by_role('button', name='来源', exact=True).click()
        self.page.get_by_role('button', name='添加来源', exact=True).click()
        kind = self.page.get_by_label('来源类型', exact=False)
        kind.select_option('rss')
        self.page.get_by_label('RSS 地址', exact=False).fill('https://example.invalid/feed')
        self.page.get_by_role('checkbox', name='允许此来源访问私网', exact=False).check()
        kind.select_option('external')
        self.page.get_by_text('高级设置：完成目录与外部入口（可稍后）', exact=True).click()
        self.page.get_by_label('浏览器中的外部应用入口', exact=False).fill('https://example.invalid/')
        kind.select_option('jmcomic')
        self.page.get_by_label('来源名称', exact=False).fill('界面切换验证漫画来源' + time.strftime('%H%M%S'))
        with self.page.expect_request('**/api/collection/sources') as submitted:
            self.page.get_by_role('button', name='保存来源', exact=True).click()
        body: dict[str, Any] = submitted.value.post_data_json
        self.check(body['kind'] == 'jmcomic' and not body.get('allowPrivate') and not body.get('externalUrl') and not body.get('url') and not body.get('path'), '来源类型切换后仅提交JMComic允许的字段')
        expect(self.page.get_by_role('dialog', name='JMComic 漫画搜索与下载')).to_be_visible()
        self.check(self.page.locator('dialog[open]').count() == 1, '保存漫画来源后单弹窗进入搜索')
        self.page.keyboard.press('Escape')
        self.page.get_by_role('navigation', name='收集分区').get_by_role('button', name='概览', exact=True).click()

    def search(self, query: str) -> None:
        """经真实应用接口提交关键词，等待明确结果或错误。"""
        self.page.locator('#jmcomic-query').fill(query)
        self.page.get_by_role('button', name='搜索漫画', exact=True).click()
        expect(self.page.get_by_text('正在搜索漫画…', exact=True)).to_be_hidden(timeout=30000)

    def layout(self, width: int, theme: str) -> None:
        """断言当前窗口和原生弹窗没有横向溢出且沿用表单内边距。"""
        self.page.set_viewport_size({'width': width, 'height': 960})
        if self.page.locator('html').get_attribute('data-theme') != theme:
            self.page.get_by_role('button', name='切换到浅色主题' if theme == 'light' else '切换到深色主题').click()
        self.open_search()
        self.search('fixture')
        metrics: dict[str, Any] = self.page.evaluate('''() => {
          const dialog = document.querySelector('dialog[open]');
          const form = dialog.querySelector('.collection-form');
          return {document: document.documentElement.scrollWidth <= innerWidth,
            dialog: dialog.scrollWidth <= dialog.clientWidth + 1,
            form: form.scrollWidth <= form.clientWidth + 1,
            padding: parseFloat(getComputedStyle(form).paddingLeft)};
        }''')
        self.check(all(metrics[key] for key in ('document', 'dialog', 'form')), f'{width}px {theme} 页面与漫画弹窗无横向溢出')
        self.check(metrics['padding'] == (18 if width <= 600 else 26), f'{width}px {theme} 沿用现有表单内边距')
        self.page.screenshot(path=str(self.output / f'jmcomic-{width}-{theme}.png'), full_page=True)
        self.page.keyboard.press('Escape')
        expect(self.page.locator('dialog[open]')).to_have_count(0)
        self.check(self.page.get_by_role('button', name='漫画搜索下载', exact=True).evaluate('(node) => node === document.activeElement'), 'Escape关闭后焦点恢复到漫画入口')

    def flow(self) -> None:
        """覆盖真实搜索分页、失败、预览失效、下载入库与实际图片阅读。"""
        self.page.set_viewport_size({'width': 1440, 'height': 1000})
        self.open_search()
        self.search('fixture')
        expect(self.page.locator('.jmcomic-results article')).to_have_count(2)
        self.page.get_by_role('button', name='下一页', exact=True).click()
        expect(self.page.get_by_text('后续页样例漫画', exact=True)).to_be_visible()
        self.check(self.page.get_by_role('button', name='下一页', exact=True).is_disabled(), '实际第二页结果禁用下一页')
        self.search('empty')
        expect(self.page.get_by_text('没有找到匹配的漫画，请更换关键词。')).to_be_visible()
        self.search('error')
        expect(self.page.locator('dialog .error-box')).to_be_visible()
        self.check(self.page.get_by_text('没有找到匹配的漫画，请更换关键词。').count() == 0, '真实接口错误不显示为空结果')
        self.search('fixture')
        self.page.get_by_role('button', name='选择并预览').first.click()
        expect(self.page.get_by_label('漫画下载预览')).to_be_visible()
        self.page.locator('#jmcomic-album').fill('1002')
        expect(self.page.get_by_label('漫画下载预览')).to_have_count(0)
        self.check(self.page.get_by_role('button', name='确认下载并入库').count() == 0, '编号改变后旧预览不能提交')
        self.page.locator('#jmcomic-album').fill('1001')
        self.page.get_by_role('button', name='预览漫画', exact=True).click()
        expect(self.page.get_by_text('漫画插件固定导入漫画库，可在入库后使用现有漫画阅读器。')).to_be_visible()
        self.page.get_by_role('button', name='确认下载并入库', exact=True).click()
        expect(self.page.get_by_role('button', name='打开媒体 1', exact=True)).to_be_visible(timeout=45000)
        self.check(self.page.locator('dialog[open]').count() == 1, '确认下载后先卸载漫画弹窗再显示任务详情')
        self.page.get_by_role('button', name='打开媒体 1', exact=True).click()
        self.page.get_by_role('button', name='开始阅读', exact=True).click()
        picture = self.page.locator('.comic-pages img').first
        expect(picture).to_be_visible()
        self.page.wait_for_function('''() => { const image = document.querySelector('.comic-pages img'); return image && image.complete && image.naturalWidth > 0 }''')
        self.page.get_by_role('button', name='下一页', exact=True).click()
        self.check('2 /' in self.page.locator('.reader-footer').inner_text(), '真实入库漫画图片可加载并翻到第二页')
        self.page.screenshot(path=str(self.output / 'jmcomic-reader.png'), full_page=True)
        self.page.keyboard.press('Escape')
        self.page.wait_for_timeout(1200)
        self.page.goto(self.page.url.split('#', 1)[0] + '#collection')
        expect(self.page.get_by_role('button', name='漫画搜索下载', exact=True)).to_be_enabled()

    def controlled_edges(self) -> None:
        """仅此方法替换响应以验证UI竞态与不确定提交，不计为后端证据。"""
        self.open_search()
        self.page.locator('#jmcomic-query').fill('slow')
        self.page.get_by_role('button', name='搜索漫画', exact=True).click()
        expect(self.page.get_by_text('正在搜索漫画…', exact=True)).to_be_visible()
        self.search('empty')
        self.page.wait_for_timeout(3500)
        expect(self.page.get_by_text('没有找到匹配的漫画，请更换关键词。')).to_be_visible()
        self.check(self.page.locator('.jmcomic-results article').count() == 0, '慢搜索迟到不能覆盖后续空结果')

        def last_page(route: Route) -> None:
            """单独模拟上游末页和极长纯文本，不把响应替换计作后端验证。"""
            route.fulfill(json={'query': 'long', 'page': 1000, 'total': 100000, 'hasNext': True, 'items': [{'albumId': '1002', 'title': '超长中文标题与标签' * 20, 'tags': ['标签' * 80]}]})

        self.page.route('**/jmcomic/search', last_page)
        try:
            self.search('long')
            self.check(self.page.get_by_role('button', name='下一页', exact=True).is_disabled(), '可控响应专项：第1000页不能再请求越界页码')
            self.page.set_viewport_size({'width': 375, 'height': 960})
            self.check(self.page.locator('.jmcomic-results').evaluate('(node) => node.scrollWidth <= node.clientWidth + 1'), '可控响应专项：长标题标签在手机结果区无横向溢出')
        finally:
            self.page.unroute('**/jmcomic/search', last_page)

        def expired_preview(route: Route) -> None:
            """仅更改真实预览的有效期，验证客户端阻止过期确认。"""
            response = route.fetch()
            body: dict[str, Any] = response.json()
            body['preview']['expiresAt'] = '2000-01-01T00:00:00Z'
            route.fulfill(response=response, json=body)

        self.page.route('**/api/collection/previews', expired_preview)
        try:
            self.page.locator('#jmcomic-album').fill('1002')
            self.page.get_by_role('button', name='预览漫画', exact=True).click()
            expect(self.page.get_by_text('预览已过期，请重新预览', exact=False)).to_be_visible()
            self.check(self.page.get_by_role('button', name='确认下载并入库', exact=True).is_disabled(), '可控响应专项：过期预览禁止新提交')
        finally:
            self.page.unroute('**/api/collection/previews', expired_preview)
        self.page.locator('#jmcomic-album').fill('1002')
        self.page.get_by_role('button', name='预览漫画', exact=True).click()
        expect(self.page.get_by_label('漫画下载预览')).to_be_visible()
        bodies: list[dict[str, Any]] = []

        def interrupt_submission(route: Route) -> None:
            """中断两次提交且保留原始请求体用于对比，不实际创建任务。"""
            bodies.append(route.request.post_data_json)
            route.abort('failed')

        self.page.route('**/api/collection/tasks', interrupt_submission)
        try:
            self.page.get_by_role('button', name='确认下载并入库', exact=True).click()
            retry = self.page.get_by_role('button', name='重试原请求', exact=True)
            expect(retry).to_be_enabled()
            self.check(self.page.locator('#jmcomic-album').is_disabled(), '提交结果不确定时锁定编号与原请求')
            retry.click()
            expect(retry).to_be_enabled()
            self.check(len(bodies) == 2 and bodies[0] == bodies[1] and bool(bodies[0]['requestId']), '可控响应专项：未知提交重试保持相同UUID与完整请求体')
        finally:
            self.page.unroute('**/api/collection/tasks', interrupt_submission)
        self.page.keyboard.press('Escape')


def main() -> None:
    """只连接本任务明确提供的隔离回环实例，关闭自有浏览器并写结果。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--info', type=Path, required=True, help='verify_jmcomic.py --serve生成的隔离browser.json')
    parser.add_argument('--browser', type=Path, default=Path('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'), help='本机受信浏览器可执行文件')
    args = parser.parse_args()
    info_file: Path = args.info.resolve()
    if not info_file.is_relative_to((ROOT / '.runtime/verify').resolve()):
        parser.error('只允许仓库.runtime/verify内的隔离服务')
    info: dict[str, str] = json.loads(info_file.read_text(encoding='utf-8'))
    base = urlsplit(info['base'])
    credential_file: Path = Path(info['credentialFile']).resolve()
    if (base.scheme != 'http' or base.hostname != '127.0.0.1' or not base.port
            or base.username or base.password or base.path or base.query or base.fragment
            or credential_file.parent != info_file.parent):
        parser.error('只允许隔离回环服务')
    credentials: dict[str, str] = json.loads(credential_file.read_text(encoding='utf-8'))
    output: Path = info_file.parent / ('browser-' + time.strftime('%H%M%S'))
    output.mkdir()
    passed: bool = False
    errors: list[str] = []
    check: BrowserCheck | None = None
    try:
        with sync_playwright() as playwright:
            browser: Browser = playwright.chromium.launch(executable_path=str(args.browser), headless=True)
            try:
                page: Page = browser.new_page(viewport={'width': 1440, 'height': 1000})
                page.on('pageerror', lambda error: errors.append(str(error)))
                page.goto(info['base'] + '/#collection')
                page.get_by_placeholder('输入你的用户名').fill(credentials['username'])
                page.locator('#auth-password').fill(credentials['password'])
                page.get_by_role('button', name='进入内容空间').click()
                expect(page.get_by_role('button', name='漫画搜索下载', exact=True)).to_be_enabled(timeout=30000)
                check = BrowserCheck(page, output)
                check.source_switch()
                for width in (375, 768, 1024, 1440):
                    for theme in ('light', 'dark'):
                        check.layout(width, theme)
                check.flow()
                check.controlled_edges()
                check.check(not errors, '浏览器无未处理页面异常')
                passed = True
            finally:
                browser.close()
    finally:
        report: dict[str, Any] = {'passed': passed, 'checks': check.assertions if check else [], 'pageErrors': errors, 'realApplication': True, 'realWorker': True, 'upstreamProtocolFixture': True, 'controlledUIResponses': True, 'pythonBridgeExecuted': False, 'realSiteSearch': False, 'realSiteDownload': False}
        (output / 'result.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
        print(json.dumps({'passed': passed, 'assertions': len(report['checks']), 'evidence': str(output)}, ensure_ascii=False), flush=True)


if __name__ == '__main__':
    raise SystemExit(run_cli(main))
