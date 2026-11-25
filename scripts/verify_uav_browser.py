"""对隔离真实应用核验UAV视频入口、布局和播放器；不访问真实视频站点。"""

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
    """保存独立断言和截图，明确区分真实链路与可控响应专项。"""

    def __init__(self, page: Page, output: Path) -> None:
        """绑定本次自有页面和仓库证据目录。"""
        self.page: Page = page  # 只操作本次新建浏览器页面。
        self.output: Path = output  # 所有结果与截图限定在.runtime。
        self.assertions: list[str] = []  # 实际成功的中文断言。

    def check(self, value: bool, description: str) -> None:
        """仅记录实际通过的断言，失败立即中止。"""
        if not value:
            raise AssertionError(description)
        self.assertions.append(description)

    def open_search(self) -> None:
        """经真实工具栏进入固定视频弹窗。"""
        self.page.get_by_role('button', name='视频搜索下载', exact=True).click()
        expect(self.page.get_by_role('dialog', name='UAV 视频搜索与下载')).to_be_visible()
        expect(self.page.locator('dialog[open]')).to_have_count(1)

    def search(self, query: str) -> None:
        """提交真实搜索请求并等待明确完成状态。"""
        self.page.locator('#uav-query').fill(query)
        self.page.get_by_role('button', name='搜索视频', exact=True).click()
        expect(self.page.get_by_text('正在搜索视频…', exact=True)).to_be_hidden(timeout=30000)

    def source_switch(self) -> None:
        """核验来源类型切换清理已隐藏的网址、目录和私网授权。"""
        self.page.get_by_role('navigation', name='收集分区').get_by_role('button', name='来源', exact=True).click()
        self.page.get_by_role('button', name='添加来源', exact=True).click()
        kind = self.page.get_by_label('来源类型', exact=False)
        kind.select_option('rss')
        self.page.get_by_label('RSS 地址', exact=False).fill('https://example.invalid/feed')
        self.page.get_by_role('checkbox', name='允许此来源访问私网', exact=False).check()
        kind.select_option('external')
        self.page.get_by_text('高级设置：完成目录与外部入口（可稍后）', exact=True).click()
        self.page.get_by_label('浏览器中的外部应用入口', exact=False).fill('https://example.invalid/')
        kind.select_option('uav')
        self.page.get_by_label('来源名称', exact=False).fill('界面切换验证视频来源' + time.strftime('%H%M%S'))
        with self.page.expect_request('**/api/collection/sources') as submitted:
            self.page.get_by_role('button', name='保存来源', exact=True).click()
        body: dict[str, Any] = submitted.value.post_data_json
        self.check(body['kind'] == 'uav' and not body.get('allowPrivate') and not body.get('externalUrl') and not body.get('url') and not body.get('path') and not body.get('rootId'), '来源切换仅提交UAV允许字段')
        expect(self.page.get_by_role('dialog', name='UAV 视频搜索与下载')).to_be_visible()
        self.check(self.page.locator('dialog[open]').count() == 1, '保存UAV来源后单弹窗进入视频搜索')
        self.page.keyboard.press('Escape')
        self.page.get_by_role('navigation', name='收集分区').get_by_role('button', name='概览', exact=True).click()

    def layout(self, width: int, theme: str) -> None:
        """检查页面与视频弹窗的换行、内边距和焦点恢复。"""
        self.page.set_viewport_size({'width': width, 'height': 960})
        if self.page.locator('html').get_attribute('data-theme') != theme:
            self.page.get_by_role('button', name='切换到浅色主题' if theme == 'light' else '切换到深色主题').click()
        self.open_search()
        self.page.get_by_label('视频站点', exact=False).select_option('supjav')
        self.search('fixture')
        metrics: dict[str, Any] = self.page.evaluate('''() => {
          const dialog = document.querySelector('dialog[open]');
          const form = dialog.querySelector('.collection-form');
          return {document: document.documentElement.scrollWidth <= innerWidth,
            dialog: dialog.scrollWidth <= dialog.clientWidth + 1,
            form: form.scrollWidth <= form.clientWidth + 1,
            padding: parseFloat(getComputedStyle(form).paddingLeft)};
        }''')
        self.check(all(metrics[key] for key in ('document', 'dialog', 'form')), f'{width}px {theme} 页面与视频弹窗无横向溢出')
        self.check(metrics['padding'] == (18 if width <= 600 else 26), f'{width}px {theme} 沿用原表单内边距')
        self.page.screenshot(path=str(self.output / f'uav-{width}-{theme}.png'), full_page=True)
        self.page.keyboard.press('Escape')
        expect(self.page.locator('dialog[open]')).to_have_count(0)
        self.check(self.page.get_by_role('button', name='视频搜索下载', exact=True).evaluate('(node) => node === document.activeElement'), 'Escape关闭后焦点恢复到视频入口')

    def flow(self) -> None:
        """经真实端点搜索、预览、入库并加载可解码视频。"""
        self.page.set_viewport_size({'width': 1440, 'height': 1000})
        self.open_search()
        unicode_query: str = '😀' * 120  # 与后端Unicode字符预算一致，不被HTML的UTF-16上限减半。
        self.search(unicode_query)
        self.check(self.page.locator('#uav-query').input_value() == unicode_query and self.page.locator('.uav-results article').count() == 2, '120个双码元Unicode字符可完整搜索')
        self.search('fixture')
        expect(self.page.locator('.uav-results article')).to_have_count(2)
        self.page.get_by_role('button', name='下一页', exact=True).click()
        expect(self.page.get_by_text('后续页样例视频', exact=True)).to_be_visible()
        self.check(self.page.get_by_role('button', name='下一页', exact=True).is_disabled(), '实际第二页结果禁用下一页')
        self.search('empty')
        expect(self.page.get_by_text('没有找到匹配的视频，请更换关键词或站点。')).to_be_visible()
        self.search('error')
        expect(self.page.locator('dialog .error-box')).to_be_visible()
        self.check(self.page.get_by_text('没有找到匹配的视频，请更换关键词或站点。').count() == 0, '搜索错误不显示为空结果')
        self.search('fixture')
        self.page.get_by_role('button', name='选择并预览').first.click()
        expect(self.page.get_by_label('视频下载预览')).to_be_visible()
        self.page.get_by_label('视频站点', exact=False).select_option('hanime1')
        expect(self.page.get_by_label('视频下载预览')).to_have_count(0)
        self.check(self.page.get_by_role('button', name='确认下载并入库').count() == 0, '切换站点清除原视频预览和确认入口')
        for site in ('missav', 'hanime1', 'supjav'):
            self.page.get_by_label('视频站点', exact=False).select_option(site)
            self.search('fixture')
            self.check(self.page.locator('.uav-results article').count() == 2, site + '通过实际界面选择与后端搜索展示结果')
        self.page.get_by_label('视频站点', exact=False).select_option('huangguo')
        self.search('fixture')
        self.check(self.page.locator('.uav-results article').count() == 2 and self.page.get_by_text('huangguo-42-ep-1', exact=False).count() > 0, '黄果选项返回规范单集结果')
        self.page.screenshot(path=str(self.output / 'uav-huangguo.png'), full_page=True)
        self.import_and_play('黄果', 'uav-player.png')
        self.open_search()
        self.page.get_by_label('视频站点', exact=False).select_option('supjav')
        self.search('fixture')
        self.check(self.page.get_by_text('编号 1101', exact=False).count() > 0, 'SupJav真实界面使用规范视频编号')
        self.page.get_by_role('button', name='下一页', exact=True).click()
        expect(self.page.get_by_text('后续页样例视频', exact=True)).to_be_visible()
        self.check(self.page.get_by_role('button', name='下一页', exact=True).is_disabled(), 'SupJav后续页按服务端结果禁用下一页')
        self.search('fixture')
        self.page.screenshot(path=str(self.output / 'uav-supjav.png'), full_page=True)
        self.import_and_play('SupJav', 'uav-supjav-player.png')

    def import_and_play(self, label: str, screenshot: str) -> None:
        """将当前结果经真实任务导入并验证原播放器，适用于各视频站点。"""
        self.page.get_by_role('button', name='选择并预览').first.click()
        expect(self.page.get_by_text('视频插件固定导入视频库，入库后使用现有播放器。')).to_be_visible()
        self.page.get_by_role('button', name='确认下载并入库', exact=True).click()
        expect(self.page.get_by_role('button', name='打开媒体 1', exact=True)).to_be_visible(timeout=60000)
        self.check(self.page.locator('dialog[open]').count() == 1, '确认后先卸载UAV弹窗再打开任务详情')
        self.page.get_by_role('button', name='打开媒体 1', exact=True).click()
        self.page.get_by_role('button', name='开始播放', exact=True).click()
        expect(self.page.locator('.video-stage video')).to_be_visible()
        self.page.wait_for_function('''() => { const media = document.querySelector('.video-stage video'); return media && media.readyState >= 2 && Number.isFinite(media.duration) && media.duration > 0 && media.videoWidth > 0 }''')
        self.check(self.page.locator('.video-stage video').evaluate('(media) => media.duration >= 1.9 && media.videoWidth === 160'), label + '样例通过真实任务入库后在原播放器加载时长和视频画面')
        self.page.screenshot(path=str(self.output / screenshot), full_page=True)
        self.page.keyboard.press('Escape')
        self.page.wait_for_timeout(1200)
        self.page.goto(self.page.url.split('#', 1)[0] + '#collection')
        expect(self.page.get_by_role('button', name='视频搜索下载', exact=True)).to_be_enabled()

    def controlled_edges(self) -> None:
        """仅本方法拦截响应，UI专项不计作真实后端或站点证据。"""
        self.open_search()
        self.page.locator('#uav-query').fill('slow')
        self.page.get_by_role('button', name='搜索视频', exact=True).click()
        expect(self.page.get_by_text('正在搜索视频…', exact=True)).to_be_visible()
        self.search('empty')
        self.page.wait_for_timeout(3500)
        expect(self.page.get_by_text('没有找到匹配的视频，请更换关键词或站点。')).to_be_visible()
        self.check(self.page.locator('.uav-results article').count() == 0, '实际迟到搜索不覆盖后续空结果')

        def last_page(route: Route) -> None:
            """单独模拟第1000页和长文本，验证分页边界与换行。"""
            route.fulfill(json={'site': 'jable', 'query': 'long', 'page': 1000, 'hasNext': True, 'items': [{'videoId': '1002', 'title': '超长中文视频标题' * 25, 'duration': '未知' * 12}]})

        self.page.route('**/uav/search', last_page)
        try:
            self.search('long')
            self.check(self.page.get_by_role('button', name='下一页', exact=True).is_disabled(), '可控响应专项：第1000页不能再请求越界页码')
            self.page.set_viewport_size({'width': 375, 'height': 960})
            self.check(self.page.locator('.uav-results').evaluate('(node) => node.scrollWidth <= node.clientWidth + 1'), '可控响应专项：长标题在手机结果区无横向溢出')
        finally:
            self.page.unroute('**/uav/search', last_page)

        def expired_preview(route: Route) -> None:
            """仅将真实预览改为过期，验证客户端阻止旧确认。"""
            response = route.fetch()
            body: dict[str, Any] = response.json()
            body['preview']['expiresAt'] = '2000-01-01T00:00:00Z'
            route.fulfill(response=response, json=body)

        self.page.route('**/api/collection/previews', expired_preview)
        try:
            self.page.get_by_role('button', name='选择并预览').first.click()
            expect(self.page.get_by_text('预览已过期，请重新预览', exact=False)).to_be_visible()
            self.check(self.page.get_by_role('button', name='确认下载并入库', exact=True).is_disabled(), '可控响应专项：过期预览禁止新提交')
        finally:
            self.page.unroute('**/api/collection/previews', expired_preview)
        self.page.get_by_role('button', name='重新预览视频', exact=True).click()
        expect(self.page.get_by_role('button', name='重新预览视频', exact=True)).to_have_count(0)
        bodies: list[dict[str, Any]] = []

        def interrupt_submission(route: Route) -> None:
            """中断提交并保留完整请求体，检验相同UUID重试。"""
            bodies.append(route.request.post_data_json)
            route.abort('failed')

        self.page.route('**/api/collection/tasks', interrupt_submission)
        try:
            self.page.get_by_role('button', name='确认下载并入库', exact=True).click()
            retry = self.page.get_by_role('button', name='重试原请求', exact=True)
            expect(retry).to_be_enabled()
            self.check(self.page.locator('#uav-query').is_disabled() and self.page.get_by_label('视频站点', exact=False).is_disabled(), '提交结果不确定时锁定搜索与站点')
            retry.click()
            expect(retry).to_be_enabled()
            self.check(len(bodies) == 2 and bodies[0] == bodies[1] and bool(bodies[0]['requestId']) and bodies[0]['contentType'] == 'video', '可控响应专项：未知提交重试保持相同UUID和完整视频请求')
        finally:
            self.page.unroute('**/api/collection/tasks', interrupt_submission)
        self.page.keyboard.press('Escape')


def main() -> None:
    """只连接明确提供的自有回环实例，启动前继承项目环境并关闭自有浏览器。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--info', type=Path, required=True, help='verify_uav.py --serve生成的隔离browser.json')
    parser.add_argument('--browser', type=Path, default=Path('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'), help='本机受信浏览器可执行路径')
    args = parser.parse_args()
    info_file: Path = args.info.resolve()
    if not info_file.is_relative_to((ROOT / '.runtime/verify').resolve()):
        parser.error('只允许仓库.runtime/verify内的隔离服务')
    info: dict[str, str] = json.loads(info_file.read_text(encoding='utf-8'))
    base = urlsplit(info['base'])
    credential_file: Path = Path(info['credentialFile']).resolve()
    if base.scheme != 'http' or base.hostname != '127.0.0.1' or not base.port or base.username or base.password or base.path or base.query or base.fragment or credential_file.parent != info_file.parent:
        parser.error('只允许本次隔离回环服务')
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
                expect(page.get_by_role('button', name='视频搜索下载', exact=True)).to_be_enabled(timeout=30000)
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
        report: dict[str, Any] = {'passed': passed, 'checks': check.assertions if check else [], 'pageErrors': errors, 'sites': ['jable', 'missav', 'hanime1', 'huangguo', 'supjav'], 'realApplication': True, 'realWorker': True, 'realFFmpeg': True, 'upstreamProtocolFixture': True, 'controlledUIResponses': True, 'pythonBridgeExecuted': False, 'realYtdlpExecuted': False, 'realSiteSearch': False, 'realSiteDownload': False}
        (output / 'result.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
        print(json.dumps({'passed': passed, 'assertions': len(report['checks']), 'evidence': str(output)}, ensure_ascii=False), flush=True)


if __name__ == '__main__':
    raise SystemExit(run_cli(main))
