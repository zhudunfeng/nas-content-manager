"""在隔离真实应用中验证漫画高级筛选、章节、批量与进度；不连接真实漫画站点。"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
import time
from typing import Any
from urllib.parse import urlsplit

from playwright.sync_api import Browser, Page, Route, expect, sync_playwright

from project_env import ROOT, run_cli
from verify_jmcomic_browser import BrowserCheck


class AdvancedCheck(BrowserCheck):
    """复用原漫画页面检查并独立标注拦截响应的界面专项。"""

    def preview_album(self, album_id: str) -> None:
        """通过正式预览接口读取目录，等待预览按钮恢复。"""
        self.page.locator('#jmcomic-album').fill(album_id)
        self.page.get_by_role('button', name='预览漫画', exact=True).click()
        expect(self.page.get_by_label('漫画下载预览')).to_be_visible(timeout=30000)
        expect(self.page.get_by_role('group', name='下载章节')).to_be_enabled()

    def filters(self) -> None:
        """验证真实请求传递白名单筛选和修改筛选后的迟到隔离。"""
        self.open_search()
        self.page.get_by_text('高级搜索筛选', exact=True).click()
        self.page.locator('#jmcomic-order').select_option('mv')
        self.page.locator('#jmcomic-time').select_option('w')
        with self.page.expect_request('**/jmcomic/search') as requested:
            self.search('fixture')
        body: dict[str, Any] = requested.value.post_data_json
        self.check(body['orderBy'] == 'mv' and body['time'] == 'w', '真实搜索请求传递浏览量与本周筛选')
        expect(self.page.locator('.jmcomic-results article')).to_have_count(2)
        self.page.locator('#jmcomic-order').select_option('tf')
        self.check(self.page.locator('.jmcomic-results article').count() == 0, '更改排序立即清除旧搜索结果')
        self.page.locator('#jmcomic-query').fill('slow')
        self.page.get_by_role('button', name='搜索漫画', exact=True).click()
        expect(self.page.get_by_text('正在搜索漫画…', exact=True)).to_be_visible()
        self.page.locator('#jmcomic-time').select_option('m')
        self.page.wait_for_timeout(3300)
        self.check(self.page.locator('.jmcomic-results article').count() == 0, '更改时间后旧慢响应不能恢复过期结果')
        self.search('empty')
        expect(self.page.get_by_text('没有找到匹配的漫画，请更换关键词。')).to_be_visible()
        self.page.keyboard.press('Escape')

    def chapter_layout(self, width: int, theme: str) -> None:
        """检查章节和批量两种内容在明暗主题各尺寸下的真实布局。"""
        self.page.set_viewport_size({'width': width, 'height': 960})
        if self.page.locator('html').get_attribute('data-theme') != theme:
            self.page.get_by_role('button', name='切换到浅色主题' if theme == 'light' else '切换到深色主题').click()
        self.open_search()
        self.preview_album('1010')
        self.page.locator('.jmcomic-chapters').screenshot(path=str(self.output / f'advanced-{width}-{theme}-chapter-fields.png'))
        for mode in ('chapters', 'batch'):
            if mode == 'batch':
                self.page.get_by_role('button', name='批量编号', exact=True).click()
                self.page.locator('#jmcomic-batch').fill('1001\n1999\n1011')
                self.page.get_by_role('button', name='预览批次', exact=True).click()
                expect(self.page.locator('[data-album-id="1011"]')).to_be_visible(timeout=30000)
                expect(self.page.get_by_role('button', name='预览批次', exact=True)).to_be_enabled()
            metrics: dict[str, bool] = self.page.evaluate('''() => {
              const dialog = document.querySelector('dialog[open]');
              const form = dialog.querySelector('.collection-form');
              return {document: document.documentElement.scrollWidth <= innerWidth,
                dialog: dialog.scrollWidth <= dialog.clientWidth + 1,
                form: form.scrollWidth <= form.clientWidth + 1};
            }''')
            self.check(all(metrics.values()), f'{width}px {theme} {mode}无横向溢出')
            self.page.screenshot(path=str(self.output / f'advanced-{width}-{theme}-{mode}.png'), full_page=True)
        focus_checks: list[bool] = []  # 连续巡航作为一个可重复的焦点检查点。
        for _ in range(12):
            self.page.keyboard.press('Tab')
            focus_checks.append(self.page.evaluate('''() => !document.hasFocus() ||
              !!document.activeElement.closest('dialog[open]')'''))
        self.check(all(focus_checks), '连续键盘巡航焦点保持在当前原生弹窗或浏览器工具栏')
        self.page.keyboard.press('Escape')
        expect(self.page.locator('dialog[open]')).to_have_count(0)
        self.check(self.page.get_by_role('button', name='漫画搜索下载', exact=True).evaluate('(node) => node === document.activeElement'), '高级页面Escape关闭后恢复入口焦点')

    def selected_flow(self) -> None:
        """真实读取超预算目录、重新绑定小章节并完成入库和图片阅读。"""
        self.page.set_viewport_size({'width': 1440, 'height': 1000})
        self.open_search()
        self.preview_album('1011')
        confirm = self.page.get_by_role('button', name='确认下载并入库', exact=True)
        self.check(confirm.is_disabled(), '超预算全本只展示目录且禁止确认')
        self.page.locator('.jmcomic-chapters input[value="12"]').uncheck()
        self.check(confirm.is_disabled(), '改变章节后旧预览禁止确认')
        with self.page.expect_request('**/api/collection/previews') as requested:
            self.page.get_by_role('button', name='更新章节预览', exact=True).click()
        expect(confirm).to_be_enabled(timeout=30000)
        self.check(requested.value.post_data_json['chapterIds'] == ['11'], '新预览只提交选中的小章节编号')
        expect(self.page.get_by_text('本次绑定 1 个章节 · 2 页', exact=True)).to_be_visible()
        confirm.click()
        expect(self.page.get_by_role('button', name='打开媒体 1', exact=True)).to_be_visible(timeout=45000)
        self.check(self.page.locator('dialog[open]').count() == 1, '选章确认后单弹窗切换到真实任务详情')
        self.page.get_by_role('button', name='打开媒体 1', exact=True).click()
        self.page.get_by_role('button', name='开始阅读', exact=True).click()
        self.page.wait_for_function('''() => { const i = document.querySelector('.comic-pages img');
          return i && i.complete && i.naturalWidth > 0 }''')
        self.page.get_by_role('button', name='下一页', exact=True).click()
        self.check('2 / 2' in self.page.locator('.reader-footer').inner_text(), '选中两页章节实际发布入库并可阅读第二页')
        self.page.screenshot(path=str(self.output / 'advanced-selected-reader.png'), full_page=True)
        self.page.keyboard.press('Escape')
        self.page.wait_for_timeout(1000)
        self.page.goto(self.page.url.split('#', 1)[0] + '#collection')
        expect(self.page.get_by_role('button', name='漫画搜索下载', exact=True)).to_be_enabled()

    def controlled_titles(self) -> None:
        """只替换展示文本，明确标记长标题换行专项而非后端目录证据。"""
        self.page.set_viewport_size({'width': 375, 'height': 960})

        def long_titles(route: Route) -> None:
            """保留真实预览身份与计数，仅模拟有界的长中文标题。"""
            response = route.fetch()
            body: dict[str, Any] = response.json()
            body['preview']['title'] = '超长漫画展示标题' * 25
            for chapter in body['preview']['jmcomic']['chapters']:
                chapter['title'] = '超长章节展示名称' * 30
            route.fulfill(response=response, json=body)

        self.page.route('**/api/collection/previews', long_titles)
        try:
            self.open_search()
            self.preview_album('1004')
            self.check(self.page.locator('.jmcomic-chapters').evaluate('(node) => node.scrollWidth <= node.clientWidth + 1'), 'UI可控文本：手机长章节标题换行且无横向溢出')
            self.page.locator('.jmcomic-chapters').screenshot(path=str(self.output / 'advanced-long-chapters.png'))
            self.page.keyboard.press('Escape')
        finally:
            self.page.unroute('**/api/collection/previews', long_titles)

    def page_progress(self) -> None:
        """观察真实worker进度响应与原生进度条，不以界面模拟充当下载证据。"""
        self.open_search()
        self.preview_album('1010')
        self.page.get_by_role('button', name='确认下载并入库', exact=True).click()
        progress = self.page.locator('dialog progress')
        expect(progress).to_be_visible(timeout=30000)
        self.page.wait_for_function('''() => { const p = document.querySelector('dialog progress');
          return p && p.value > 0 && p.value < p.max && p.max === 8 }''', timeout=30000)
        self.check(True, '真实获取期间原生progress显示非零且未完成的8页进度')
        self.page.screenshot(path=str(self.output / 'advanced-page-progress.png'), full_page=True)
        expect(self.page.get_by_role('button', name='打开媒体 1', exact=True)).to_be_visible(timeout=45000)
        self.check(True, '进度完成后仍经过真实发布和漫画库入库')
        self.page.keyboard.press('Escape')

    def batch_edges(self) -> None:
        """实际预览与部分受理，单独拦截未知提交验证完整原请求重试。"""
        self.open_search()
        self.page.get_by_role('button', name='批量编号', exact=True).click()
        batch = self.page.locator('#jmcomic-batch')
        batch.fill('\n'.join(str(value) for value in range(2001, 2012)))
        self.page.get_by_role('button', name='预览批次', exact=True).click()
        self.check(self.page.locator('[data-album-id]').count() == 0, '超过10本的批次在预览前被拒绝')
        batch.fill('1001,1001')
        self.page.get_by_role('button', name='预览批次', exact=True).click()
        self.check(self.page.locator('[data-album-id]').count() == 0, '重复漫画编号在预览前被拒绝')
        batch.fill('1001\n1999\n1002\n1011')
        self.page.get_by_role('button', name='预览批次', exact=True).click()
        expect(self.page.get_by_role('button', name='确认下载 2 本', exact=True)).to_be_enabled(timeout=30000)
        expect(self.page.locator('[data-album-id="1999"] .error-box')).to_be_visible()
        self.check(True, '单本预览失败和超预算目录不阻止其余两本确认')
        submitted: list[dict[str, Any]] = []

        def partial_submission(route: Route) -> None:
            """仅第二本中断响应，第一本通过真实服务受理并留在批次中。"""
            submitted.append(route.request.post_data_json)
            if len(submitted) == 1:
                route.continue_()
            else:
                route.abort('failed')

        self.page.route('**/api/collection/tasks', partial_submission)
        try:
            self.page.get_by_role('button', name='确认下载 2 本', exact=True).click()
            retry = self.page.get_by_role('button', name='重试未受理条目 1 本', exact=True)
            expect(retry).to_be_enabled(timeout=30000)
            expect(self.page.locator('[data-album-id="1001"]').get_by_role('button', name='查看收集任务', exact=True)).to_be_visible()
            self.check(self.page.locator('#jmcomic-source').is_disabled() and batch.is_disabled(), '部分提交后来源与批次被锁定且已受理任务可查看')
            self.check(self.page.get_by_role('button', name='搜索与选章', exact=True).is_disabled(), '未知结果不能通过切换模式丢弃冻结请求')
            # 仅控制浏览器时间，确认未知原请求不受客户端预览过期门禁阻断。
            self.page.evaluate('Date.now = () => 4102444800000')
            self.page.wait_for_timeout(1200)
            retry.click()
            expect(retry).to_be_enabled()
            self.check(len(submitted) == 3 and submitted[1] == submitted[2] and submitted[0]['requestId'] != submitted[1]['requestId'], 'UI可控异常：过期后未知条目复用完整UUID/body且成功条目不重发')
        finally:
            self.page.unroute('**/api/collection/tasks', partial_submission)
            self.page.evaluate('delete Date.now; Date.now = () => new Date().getTime()')
        self.page.screenshot(path=str(self.output / 'advanced-batch-partial.png'), full_page=True)
        self.page.keyboard.press('Escape')


def main() -> None:
    """限制为本任务新建的隔离回环服务，关闭自有浏览器并持久化真实结果。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--info', type=Path, required=True, help='高级协议脚本--serve生成的隔离browser.json')
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
    output: Path = info_file.parent / ('advanced-browser-' + time.strftime('%H%M%S'))
    output.mkdir()
    errors: list[str] = []
    passed: bool = False
    check: AdvancedCheck | None = None
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
                check = AdvancedCheck(page, output)
                check.filters()
                for width in (375, 768, 1024, 1440):
                    for theme in ('light', 'dark'):
                        check.chapter_layout(width, theme)
                check.controlled_titles()
                check.selected_flow()
                check.page_progress()
                check.batch_edges()
                check.check(not errors, '浏览器无未处理页面异常')
                passed = True
            finally:
                if check is not None and not passed:
                    page.screenshot(path=str(output / 'failure.png'), full_page=True)
                browser.close()
    finally:
        report: dict[str, Any] = {'passed': passed, 'checks': check.assertions if check else [], 'pageErrors': errors, 'realApplication': True, 'realWorker': True, 'upstreamProtocolFixture': True, 'controlledUIResponses': True, 'pythonBridgeExecuted': False, 'realSiteSearch': False, 'realSiteDownload': False}
        (output / 'result.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
        print(json.dumps({'passed': passed, 'assertions': len(report['checks']), 'evidence': str(output)}, ensure_ascii=False), flush=True)


if __name__ == '__main__':
    raise SystemExit(run_cli(main))
