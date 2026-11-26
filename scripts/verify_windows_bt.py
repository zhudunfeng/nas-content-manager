"""通过真实 Windows 桌面进程验证 bt.json 与受控搜种下载闭环，不执行 Docker 或真实 Peer 传输。"""

from __future__ import annotations

import sys

if __name__ == '__main__':
    sys.dont_write_bytecode = True

import argparse
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
from typing import Any

from project_env import verification_directory
from verify import generate_password
from verify_desktop import control
from verify_torrent_search import SearchHarness


class WindowsBTHarness(SearchHarness):
    """复用本机协议样本，以桌面 profile 取代下载及搜索环境参数。"""

    def __init__(self, binary: Path, root: Path) -> None:
        """准备中文空格路径、私密配置及大小写不同的引擎回报。"""
        super().__init__(binary, root)
        # 桌面配置目录与工作目录分离，验证双击和从其他目录启动的稳定引用。
        self.profile: Path = root / '原生配置 中文'
        self.profile.mkdir()
        # 搜种和密码只写本次隔离 profile，不读取任何用户引擎配置。
        private: Path = self.profile / 'bt'
        private.mkdir()
        (private / 'qb-password').write_text(self.engine.password, encoding='utf-8-sig')
        sources: str = (root / 'sources.json').read_text(encoding='utf-8-sig')
        (private / 'sources.json').write_text(sources, encoding='utf-8-sig')
        # 只改变回报字符串；实际样本仍写 root/downloads，绝不写入虚拟 E 盘位置。
        self.engine.windows_engine_paths = True
        self.engine.engine_root = 'E:/BT 中文下载'
        # 可重复切换搜索/下载状态的非敏感 profile 文档。
        self.bt: dict[str, Any] = {
            'schemaVersion': 1,
            'downloads': {
                'mode': 'external',
                'root': str(root / 'downloads'),
                'engineRoot': self.engine.engine_root,
                'url': self.engine.base,
                'username': 'verify',
                'passwordFile': 'bt/qb-password',
            },
            'search': {'mode': 'external', 'sourcesFile': 'bt/sources.json'},
        }
        self.save_bt()
        # 移除继承或基类的引擎环境，确保本轮通过 bt.json 真正启用模块。
        for key in list(self.env):
            if key.startswith(('NAS_DOWNLOAD_', 'NAS_TORRENT_SEARCH_', 'NAS_ENGINE_', 'NAS_COLLECTION_')) or key in ('NAS_ADDR', 'NAS_DATA_DIR', 'NAS_MEDIA_ROOTS'):
                del self.env[key]
        self.env['NAS_DOWNLOAD_MIN_FREE_GB'] = '0'  # 小样本回归不额外预留真实载荷空间。

    def save_bt(self) -> None:
        """以带 BOM 的 UTF-8 保存配置，覆盖 Windows PowerShell 常见输出。"""
        (self.profile / 'bt.json').write_text(json.dumps(self.bt, ensure_ascii=False), encoding='utf-8-sig')

    def start(self) -> None:
        """从不同工作目录启动真实桌面 EXE，等待原应用健康检查。"""
        self.log = (self.root / 'application.log').open('ab')
        arguments: list[str] = [str(self.binary), '--headless', '--no-browser', '--collection-mode', 'disabled', '--profile', str(self.profile), '--data-dir', str(self.root / 'data'), '--media-root', str(self.root / 'media'), '--port', self.base.rsplit(':', 1)[1]]
        self.process = subprocess.Popen(arguments, env=self.env, cwd=self.root / 'media', stdout=self.log, stderr=subprocess.STDOUT)
        deadline: float = time.monotonic() + 30
        while time.monotonic() < deadline:
            if self.process.poll() is not None:
                raise AssertionError('Windows BT 桌面实例提前退出，请检查隔离日志')
            try:
                if self.client.request('/healthz')[0] == 200:
                    return
            except OSError:
                pass
            time.sleep(.15)
        raise AssertionError('Windows BT 桌面实例启动超时')

    def stop(self) -> None:
        """经当前用户私有 IPC 有序退出，并核对外接样本服务继续运行。"""
        try:
            if self.process and self.process.poll() is None:
                control(self.binary, self.profile, 'quit', self.env, self.root / 'media')
                self.process.wait(timeout=35)
                if self.process.returncode != 0:
                    raise AssertionError('Windows BT 桌面实例退出失败')
        finally:
            if self.process and self.process.poll() is None:
                self.process.kill()
                self.process.wait(timeout=10)
            if self.log:
                self.log.close()
                self.log = None

    def run(self) -> None:
        """验证持久接入、搜索选集入库、重启幂等、独立停用与凭据隔离。"""
        self.start()
        client = self.client
        client.json('/api/setup', 'POST', {'username': 'windows-bt-verify', 'password': generate_password()})
        client.json('/api/downloads/connection-test', 'POST', {})
        status: dict[str, Any] = client.json('/api/engines')
        client.check(all(component['mode'] == 'external' and not component['canInitialize'] for component in status['components']), 'bt.json 原生外接配置生效，不开放 Docker 初始化')
        client.check(client.json('/api/torrent-search/status')['enabled'], '来源文件通过 profile 相对路径读取，允许 UTF8 BOM')
        # torrent 关键词只返回受控 HTTP 种子代理样本。
        results: list[dict[str, Any]] = self.search('torrent')['results']
        candidate: dict[str, Any] = results[0]
        request_id: str = secrets.token_hex(16)
        first: dict[str, Any] = self.download(candidate, request_id)['download']
        task: dict[str, Any] = self.wait_task(first['id'], lambda item: item['state'] == 'awaiting_selection')
        client.check(not task['ready'] and not task['selectionLocked'], 'Windows 下载先等待选集，不提前发布')
        self.select(task, [0])
        self.wait_task(task['id'], lambda item: item['state'] == 'downloading')
        self.engine.complete(self.fixtures['torrent'].hash)
        published: dict[str, Any] = self.wait_task(task['id'], lambda item: item['importState'] == 'imported', timeout=60)
        items: list[dict[str, Any]] = client.json('/api/downloads/' + task['id'] + '/items')['items']
        client.check(published['ready'] and len(items) == 1 and client.request('/api/items/' + str(items[0]['id']) + '/file')[2] == self.fixtures['torrent'].files[0][1], 'Windows 盘符大小写及反斜杠回报完成选集、移动、入库与原字节读取')
        client.check(len(client.json('/api/libraries')['libraries']) == 1, 'bt.json 只授权 complete 子树即可自动创建受管库')
        self.stop()
        client.check(self.engine.thread.is_alive() and self.indexer.thread.is_alive(), '桌面退出没有停止或接管外接引擎')
        fetched: int = self.indexer.downloads
        self.start()
        client.json('/api/downloads/connection-test', 'POST', {})
        replay: dict[str, Any] = self.download(candidate, request_id)['download']
        client.check(replay['id'] == first['id'] and self.indexer.downloads == fetched, '桌面重启保留原账号、下载记录与幂等请求')
        self.stop()
        self.bt['downloads']['mode'] = 'disabled'
        self.save_bt()
        self.start()
        client.check(client.json('/api/torrent-search/status')['enabled'], '关闭下载后搜种仍独立可用')
        state: dict[str, Any] = client.json('/api/engines')
        client.check(next(component for component in state['components'] if component['id'] == 'qbittorrent')['mode'] == 'disabled', '下载协调可独立关闭')
        client.check(client.request('/api/items/' + str(items[0]['id']) + '/file')[2] == self.fixtures['torrent'].files[0][1], '停用下载保留历史门禁和已发布文件访问')
        self.stop()
        self.bt['search']['mode'] = 'disabled'
        self.save_bt()
        self.start()
        requested: int = len(self.indexer.requests)
        client.check(not client.json('/api/torrent-search/status')['enabled'] and client.request('/api/torrent-search', 'POST', {'query': '停止搜索'})[0] == 503 and requested == len(self.indexer.requests), '关闭搜种不访问来源且返回明确停用状态')
        self.stop()
        log: str = (self.root / 'application.log').read_text(encoding='utf-8')
        client.check(self.engine.password not in log and self.indexer.key not in log and self.engine.base not in log and self.indexer.base not in log, '启动及业务日志不泄露引擎凭据或私密地址')
        print(f'Windows 原生 BT 集成通过：{client.passed} 项断言；真实桌面 EXE + 受控 Torznab/qB，无 Docker、公共站点或 Peer 传输。', flush=True)


def main() -> None:
    """仅允许 Windows 真实桌面二进制及全新的项目内隔离目录。"""
    parser: argparse.ArgumentParser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True, type=Path, help='本次构建的 Windows 桌面 EXE')
    parser.add_argument('--root', type=Path, default=verification_directory() / ('windows-bt 中文-' + time.strftime('%Y%m%d-%H%M%S') + '-' + secrets.token_hex(3)), help='全新且包含 verify 段的隔离路径')
    args: argparse.Namespace = parser.parse_args()
    binary: Path = args.binary.resolve()
    root: Path = args.root.resolve()
    if os.name != 'nt' or not binary.is_file() or root.exists() or 'verify' not in root.parts:
        parser.error('需要 Windows、已构建桌面 EXE 与全新 verify 隔离目录')
    root.mkdir(parents=True)
    harness: WindowsBTHarness = WindowsBTHarness(binary, root)
    passed: bool = False
    try:
        harness.run()
        passed = True
    finally:
        harness.close()
        (root / 'result.json').write_text(json.dumps({'passed': passed, 'assertions': harness.client.passed, 'realWindowsDesktop': True, 'providers': 'controlled Torznab and qB', 'dockerExecuted': False, 'realSiteSearch': False, 'realPeerTransfer': False}, ensure_ascii=False, indent=2), encoding='utf-8')


if __name__ == '__main__':
    from project_env import run_cli

    raise SystemExit(run_cli(main))
