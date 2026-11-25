"""以真实应用与worker验证高级漫画协议；受控Go样例不执行Python或访问真实站点。"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
import sqlite3
import sys
import time
from typing import Any
import uuid

sys.dont_write_bytecode = True

from verify_jmcomic import JMComicHarness, main as run_verifier


class AdvancedJMComicHarness(JMComicHarness):
    """复用原隔离生命周期和阅读断言，补充跨层高级能力验证。"""

    def fixture_calls(self, action: str, album_id: str = '') -> list[dict[str, Any]]:
        """读取本次受控工具的脱敏业务字段，不记录代理、查询词或路径。"""
        path: Path = self.fixture_dir / 'calls.jsonl'  # 当前验证独占的协议调用记录。
        calls: list[dict[str, Any]] = [json.loads(line) for line in path.read_text(encoding='utf-8').splitlines() if line] if path.exists() else []  # 只含固定安全字段。
        return [call for call in calls if call['action'] == action and (not album_id or call['albumId'] == album_id)]

    def task_body(self, preview: dict[str, Any]) -> dict[str, Any]:
        """生成不可变任务提交体，选章只从服务端预览继承。"""
        return {'requestId': str(uuid.uuid4()), 'previewId': preview['id'], 'contentType': 'comic', 'createLibrary': True, 'libraryId': 0}

    def verify_search_filters(self, source: dict[str, Any]) -> None:
        """验证每个排序与时间枚举真正传到受控桥接，并拒绝越界输入。"""
        endpoint: str = '/api/collection/sources/' + source['id'] + '/jmcomic/search'  # 当前来源的固定入口。
        for order_by in ('mr', 'mv', 'mp', 'tf'):
            for period in ('a', 't', 'w', 'm'):
                response = self.client.json(endpoint, 'POST', {'query': 'advanced', 'page': 1, 'orderBy': order_by, 'time': period})
                last = self.fixture_calls('search')[-1]
                self.client.check(response['page'] == 1 and len(response['items']) == 2, f'高级搜索接受排序{order_by}与时间{period}')
                self.client.check(last['orderBy'] == order_by and last['time'] == period, f'排序{order_by}与时间{period}实际传递到固定桥接协议')
        legacy = self.client.json(endpoint, 'POST', {'query': 'fixture', 'page': 2})
        self.client.check(legacy['query'] == 'fixture' and legacy['page'] == 2 and legacy['items'][0]['albumId'] == '1003', '省略新增筛选时保留旧关键词、分页和结果协议')
        last = self.fixture_calls('search')[-1]
        self.client.check(last['orderBy'] in ('', 'mr') and last['time'] in ('', 'a'), '旧请求仍采用默认mr/a筛选语义')
        default_page = self.client.json(endpoint, 'POST', {'query': '  中文关键词  ', 'page': 0})
        self.client.check(default_page['query'] == '中文关键词' and default_page['page'] == 1, '旧请求仍规范化中文关键词与默认页码')
        for field, values in (('orderBy', ('MR', 'latest', '../x', 'x' * 20)), ('time', ('year', '1w', '../x', 'A'))):
            for value in values:
                status = self.client.request(endpoint, 'POST', {'query': 'fixture', 'page': 1, field: value})[0]
                self.client.check(status == 400, f'高级搜索拒绝非法{field}值')
        for extra in ({'category': '1'}, {'subCategory': '1'}, {'query': 'x' * 121}, {'chapterIds': ['11']}):
            self.client.check(self.client.request(endpoint, 'POST', dict({'query': 'fixture', 'page': 1}, **extra))[0] == 400, '搜索继续拒绝无效分类、超长查询和无关参数')
        self.client.check(self.worker_request('/v1/jmcomic/search', 'POST', {'query': 'fixture', 'page': 1, 'orderBy': 'invalid'})[0] == 400, 'worker独立拒绝非法排序而不依赖应用校验')

    def verify_chapter_previews(self, source: dict[str, Any]) -> None:
        """验证目录、全本兼容、规范化顺序及无效选择不会创建操作。"""
        whole = self.preview(source, '1001')
        empty = self.preview(source, '1001', [])
        metadata: dict[str, Any] = whole['jmcomic']  # 公开目录只含固定字段。
        self.client.check(metadata['albumId'] == '1001' and metadata['pageCount'] == 4 and metadata['selectedChapterIds'] == ['11', '12'], '省略章节选择默认全本并展示所选总页数')
        self.client.check([chapter['id'] for chapter in metadata['chapters']] == ['11', '12'] and [chapter['pageCount'] for chapter in metadata['chapters']] == [2, 2] and all(chapter['title'] for chapter in metadata['chapters']), '预览保留完整有序章节标题与各章真实页数')
        self.client.check(empty['jmcomic'] == metadata and not metadata.get('overBudget', False), '显式空章节选择与省略字段具有相同全本含义')
        _, operations_before = self.worker_request('/v1/operations')
        for selection in (['11', '11'], ['999'], ['11', '999'], ['../11'], ['01']):
            status = self.client.request('/api/collection/previews', 'POST', {'sourceId': source['id'], 'albumId': '1001', 'chapterIds': selection})[0]
            self.client.check(status in (400, 422), '重复、未知或非法章节选择被明确拒绝')
        _, operations_after = self.worker_request('/v1/operations')
        self.client.check(operations_before == operations_after, '无效章节预览不会产生worker持久操作')
        worker_input: dict[str, Any] = {'kind': 'jmcomic', 'profile': 'comic', 'albumId': '1001', 'maxBytes': 20971520, 'maxFiles': 100}  # 与默认应用预算一致。
        status, legacy = self.worker_request('/v1/previews', 'POST', worker_input)
        _, empty_worker = self.worker_request('/v1/previews', 'POST', dict(worker_input, chapterIds=[]))
        old_identity: str = hashlib.sha256(json.dumps({'albumId': '1001', 'title': '', 'pageCount': 4, 'chapters': ['11', '12'], 'files': 0}, separators=(',', ':')).encode()).hexdigest()  # 旧结构的固定字段顺序。
        self.client.check(status == 200 and legacy['identity'] == old_identity and empty_worker['identity'] == old_identity, '默认全本及空选择保持旧版元数据身份摘要兼容')
        _, subset = self.worker_request('/v1/previews', 'POST', dict(worker_input, chapterIds=['12']))
        self.client.check(subset['jmcomic']['pageCount'] == 2 and subset['identity'] != legacy['identity'], '明确选章生成独立身份并只计算所选章节总页数')
        operation_id: str = str(uuid.uuid4())  # 只用于被拒绝的伪造操作。
        forged = dict(worker_input, id=operation_id, previewId=subset['id'], chapterIds=['11'])
        self.client.check(self.worker_request('/v1/operations', 'POST', forged)[0] == 409, 'worker拒绝用已冻结预览替换为其他章节')
        self.client.check(self.worker_request('/v1/operations/' + operation_id)[0] == 404, '伪造章节操作不会占用幂等编号或留下历史记录')

    def watch_selected_task(self, task_id: str, expected_pages: int) -> tuple[dict[str, Any], list[dict[str, Any]]]:
        """观察真实应用页面快照并校验单次获取内的单调进度与修订稳定。"""
        deadline: float = time.monotonic() + 90  # 有界等待发布和实际扫描。
        snapshots: list[dict[str, Any]] = []  # 仅保存安全字段，用于独立复核。
        task: dict[str, Any] = {}  # 最后一次真实应用事实。
        while time.monotonic() < deadline:
            task = self.client.json('/api/collection/tasks/' + task_id)['task']
            progress = task.get('jmcomicProgress')
            if progress is not None:
                if set(progress) != {'completedPages', 'totalPages', 'bytes', 'reusedPages'}:
                    raise AssertionError('页面进度出现未约定或可能敏感的字段')
                if any(type(progress[key]) is not int for key in progress):
                    raise AssertionError('页面进度必须为有界整数')
                if not (0 <= progress['completedPages'] <= expected_pages == progress['totalPages'] and 0 <= progress['reusedPages'] <= progress['completedPages'] and 0 <= progress['bytes'] <= 20971520):
                    raise AssertionError('页面进度超出冻结的选择或预算')
                if snapshots and any(progress[key] < snapshots[-1][key] for key in ('completedPages', 'bytes', 'reusedPages')):
                    raise AssertionError('同一次获取的页面进度发生回退')
                snapshots.append(dict(progress, state=task['state'], revision=task['revision']))
            if task['state'] == 'failed' or task['importState'] == 'imported':
                break
            time.sleep(.1)
        self.client.check(task.get('importState') == 'imported', '所选章节经过真实获取、完整性校验、发布和扫描后完成入库')
        self.client.check(len({snapshot['completedPages'] for snapshot in snapshots}) >= 3 and any(0 < snapshot['completedPages'] < expected_pages for snapshot in snapshots), '运行期间实际观察到多个不同且未完成的页面进度')
        self.client.check(bool(snapshots) and all(snapshot['totalPages'] == expected_pages and snapshot['reusedPages'] == 0 for snapshot in snapshots), '协议样例快照保持冻结总页数，未冒充真实Python缓存复用')
        active_revisions: set[int] = {snapshot['revision'] for snapshot in snapshots if snapshot['state'] == 'acquiring'}  # 页面进度不应逐页写入业务修订。
        self.client.check(len(active_revisions) == 1, '获取中的逐页进度不反复修改持久任务操作版本')
        return task, snapshots

    def verify_selected_publication(self, task: dict[str, Any], expected_paths: set[str], album_id: str = '1010') -> dict[str, Any]:
        """核对最终清单、真实文件、字节总数和媒体目录外的私密进度。"""
        status, operation = self.worker_request('/v1/operations/' + task['id'])
        self.client.check(status == 200 and operation['state'] == 'published', '真实worker只在完整所选页面核验后标记发布')
        self.client.check({file['path'] for file in operation['files']} == expected_paths, '最终发布清单只包含所选章节的完整固定路径')
        directory: Path = self.worker_root / operation['relativeDir']  # 真实worker已验证的发布相对根。
        actual: set[str] = {path.relative_to(directory).as_posix() for path in directory.rglob('*') if path.is_file()}  # 不忽略任何额外控制文件。
        total_bytes: int = sum((directory / path).stat().st_size for path in expected_paths)  # 来自实际图片文件的最终字节总数。
        self.client.check(actual == expected_paths and operation['bytes'] == total_bytes and task['bytes'] == total_bytes, '无未选中或控制文件进入发布，最终字节数不重复累计实时进度')
        self.client.check(sum(file['size'] for file in operation['files']) == total_bytes, '发布清单逐文件长度与操作及任务的最终字节事实一致')
        checkpoint: Path = self.worker_root / 'private-state' / 'jmcomic' / task['id'] / 'progress.json'  # 进度只允许出现在私密子目录。
        progress: dict[str, Any] = json.loads(checkpoint.read_text(encoding='utf-8'))
        self.client.check(set(progress) == {'completedPages', 'totalPages', 'bytes', 'reusedPages'} and progress['completedPages'] == len(expected_paths) and progress['bytes'] == total_bytes, '四字段进度留在任务专属私密子目录并与最终真实页面一致')
        calls = self.fixture_calls('download', album_id)
        self.client.check(bool(calls) and all(call['hasCheckpoint'] and call['hasBinding'] for call in calls), '真实worker为下载传递任务专属checkpoint与预览绑定摘要')
        self.verify_reader(task, len(expected_paths))
        return operation

    def verify_over_budget(self, source: dict[str, Any]) -> None:
        """证明超预算目录只能展示，减少选章后才可创建真实获取任务。"""
        preview = self.preview(source, '1011')
        metadata = preview['jmcomic']
        self.client.check(metadata['overBudget'] and metadata['pageCount'] == 102 and len(metadata['chapters']) == 2, '超预算全本仍返回有界完整目录以便减少选择')
        blocked_body = self.task_body(preview)
        self.client.check(self.client.request('/api/collection/tasks', 'POST', blocked_body)[0] == 422 and self.download_count('1011') == 0, '应用禁止从超预算全本预览创建下载任务')
        worker_input = {'kind': 'jmcomic', 'profile': 'comic', 'albumId': '1001', 'maxBytes': 20971520, 'maxFiles': 3}
        status, worker_preview = self.worker_request('/v1/previews', 'POST', worker_input)
        self.client.check(status == 200 and worker_preview['jmcomic']['overBudget'] and worker_preview['jmcomic']['pageCount'] == 4, 'worker保留低预算全本预览的只读章节目录')
        operation_id: str = str(uuid.uuid4())  # 超预算拒绝不保存该编号。
        self.client.check(self.worker_request('/v1/operations', 'POST', dict(worker_input, id=operation_id, previewId=worker_preview['id']))[0] == 422, 'worker独立拒绝超预算预览创建获取操作')
        self.client.check(self.worker_request('/v1/operations/' + operation_id)[0] == 404, '超预算拒绝不会留下可执行的worker操作')
        subset = self.preview(source, '1011', ['11'])
        self.client.check(subset['jmcomic']['pageCount'] == 2 and not subset['jmcomic'].get('overBudget', False), '减少为小章节后重新预览解除超预算状态')
        task = self.client.json('/api/collection/tasks', 'POST', self.task_body(subset))['task']
        task = self.wait_collection(task['id'], lambda item: item['state'] == 'failed' or item['importState'] == 'imported')
        self.verify_selected_publication(task, {'00001/00001.jpg', '00001/00002.jpg'}, '1011')

    def verify_full_chapter_drift(self) -> None:
        """保持旧全本身份兼容时，逐章页数漂移仍必须独立拒绝获取成功。"""
        worker_input: dict[str, Any] = {'kind': 'jmcomic', 'profile': 'comic', 'albumId': '1012', 'maxBytes': 20971520, 'maxFiles': 100}  # 故意省略选章以覆盖旧全本身份。
        status, original = self.worker_request('/v1/previews', 'POST', worker_input)
        self.client.check(status == 200 and [chapter['pageCount'] for chapter in original['jmcomic']['chapters']] == [2, 2], '目录漂移样例先冻结默认全本两章各两页的预览')
        flag: Path = self.fixture_dir / 'metadata-drift'  # 仅控制本次1012样例，不访问外部元数据。
        flag.write_text('controlled chapter page-count drift', encoding='utf-8')
        try:
            status, changed = self.worker_request('/v1/previews', 'POST', worker_input)
            self.client.check(status == 200 and changed['jmcomic']['pageCount'] == original['jmcomic']['pageCount'] == 4 and changed['identity'] == original['identity'] and [chapter['pageCount'] for chapter in changed['jmcomic']['chapters']] == [1, 3], '全本总页数与旧身份不变时，实际章节目录已从2+2漂为1+3')
            operation_id: str = str(uuid.uuid4())  # 原始预览仍必须通过执行时的独立目录核对。
            status, operation = self.worker_request('/v1/operations', 'POST', dict(worker_input, id=operation_id, previewId=original['id']))
            self.client.check(status == 202, '使用原冻结预览提交真实worker操作以验证获取边界')
            deadline: float = time.monotonic() + 20  # 只等待本次真实操作到稳定终态。
            while operation.get('state') not in ('failed', 'acquired', 'published', 'canceled') and time.monotonic() < deadline:
                time.sleep(.1)
                _, operation = self.worker_request('/v1/operations/' + operation_id)
            self.client.check(operation.get('state') == 'failed' and operation.get('files') == [] and operation.get('bytes') == 0, '即使总页数和旧摘要相同，逐章页数漂移仍使真实获取失败')
            self.client.check(not (self.worker_root / 'complete' / operation_id).exists(), '同总页数的分章漂移不能产生任何已发布漫画目录')
        finally:
            flag.unlink(missing_ok=True)

    def run_jmcomic(self) -> None:
        """执行高级协议端到端验证，保留可复核且明确标注边界的证据。"""
        source = self.initialize()
        self.verify_search_filters(source)
        self.verify_chapter_previews(source)
        preview = self.preview(source, '1010', ['13', '11'])
        self.client.check(preview['jmcomic']['selectedChapterIds'] == ['11', '13'] and preview['jmcomic']['pageCount'] == 5 and len(preview['jmcomic']['chapters']) == 3, '逆序明确选章规范为上游顺序，同时仍保留完整三章目录')
        body = self.task_body(preview)
        self.client.check(self.client.request('/api/collection/tasks', 'POST', dict(body, chapterIds=['12']))[0] == 400, '公开任务接口拒绝脱离服务端冻结预览另传章节')
        task = self.client.json('/api/collection/tasks', 'POST', body)['task']
        self.client.check(self.client.json('/api/collection/tasks', 'POST', body)['task']['id'] == task['id'], '相同章节预览和UUID重试始终返回同一持久任务')
        different = self.preview(source, '1010', ['12'])
        self.client.check(self.client.request('/api/collection/tasks', 'POST', dict(body, previewId=different['id']))[0] == 409, '相同请求UUID不能改绑另一个章节选择')
        self.client.check(self.client.request('/api/collection/tasks', 'POST', dict(body, requestId=str(uuid.uuid4())))[0] == 409, '同一章节选择的新UUID仍识别为重复资源')
        imported, snapshots = self.watch_selected_task(task['id'], 5)
        paths: set[str] = {'00001/00001.jpg', '00001/00002.jpg', '00003/00001.jpg', '00003/00002.jpg', '00003/00003.jpg'}  # 故意跳过第二章。
        self.verify_selected_publication(imported, paths)
        self.client.check(self.download_count('1010') == 1, '章节选择的未知响应重试不会触发第二次真实协议获取')
        other_body = self.task_body(different)
        other = self.client.json('/api/collection/tasks', 'POST', other_body)['task']
        other, second_snapshots = self.watch_selected_task(other['id'], 3)
        self.verify_selected_publication(other, {'00002/00001.jpg', '00002/00002.jpg', '00002/00003.jpg'})
        self.client.check(other['id'] != task['id'] and self.download_count('1010') == 2, '同一本的不同章节选择拥有独立身份和完整发布目录')
        with sqlite3.connect(self.root / 'data/library.db') as database:
            database.execute('UPDATE collection_previews SET expires_at=? WHERE id=?', ('2000-01-01T00:00:00Z', preview['id']))
        self.client.check(self.client.json('/api/collection/tasks', 'POST', body)['task']['id'] == task['id'], '已受理提交在预览过期后仍用完全相同body确认原任务')
        self.verify_over_budget(source)
        self.verify_full_chapter_drift()
        public = json.dumps({'task': self.client.json('/api/collection/tasks/' + task['id']), 'preview': preview}, ensure_ascii=False)
        token: str = self.token_file.read_text(encoding='utf-8').strip()  # 仅内存比较，不写入共享证据。
        self.client.check(token not in public and self.credentials['password'] not in public and all(field not in public for field in ('checkpointDir', 'outputDir', 'binding', 'proxy')), '高级公开元数据与页面进度不泄露令牌、路径或私密绑定')
        self.stop()
        self.stop_worker()
        self.start_worker()
        self.start()
        self.client.json('/api/login', 'POST', self.credentials)
        restored = self.client.json('/api/collection/tasks/' + task['id'])['task']
        self.client.check(restored['importState'] == 'imported' and restored['itemIds'] == imported['itemIds'], '包含私密进度子目录的worker与应用重启后仍恢复原发布关联')
        self.client.check(self.client.json('/api/collection/tasks', 'POST', body)['task']['id'] == task['id'] and self.download_count('1010') == 2, '重启并过期后相同不可变提交不会重复获取任意章节')
        logs: str = '\n'.join(path.read_text(encoding='utf-8', errors='replace') for path in (self.root / 'worker.log', self.root / 'application.log') if path.exists())  # 只在本地检查，不回显内容。
        self.client.check(token not in logs and self.credentials['password'] not in logs, '高级下载和重启日志不包含测试凭据或worker令牌')
        evidence: dict[str, Any] = {'realApplication': True, 'realWorker': True, 'pythonBridgeExecuted': False, 'upstreamProtocolFixture': True, 'realSiteSearch': False, 'realSiteDownload': False, 'dockerExecuted': False, 'firstSelection': ['11', '13'], 'secondSelection': ['12'], 'progressSnapshots': snapshots, 'secondProgressSnapshots': second_snapshots, 'assertions': self.client.passed}  # 明确区分协议样例与真实依赖/站点证据。
        (self.root / 'advanced-evidence.json').write_text(json.dumps(evidence, ensure_ascii=False, indent=2), encoding='utf-8')
        print(f'高级漫画协议验证通过：{self.client.passed}项；真实应用与worker，Go协议样例；Python缓存与真实站点由独立边界另验。', flush=True)


def main() -> None:
    """沿用安全隔离CLI，保留独立serve模式供高级浏览器验证。"""
    run_verifier(AdvancedJMComicHarness, 'jmcomic-advanced')


if __name__ == '__main__':
    from project_env import run_cli

    raise SystemExit(run_cli(main))
