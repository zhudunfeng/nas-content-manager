"""用真实主服务与获取执行端验证漫画插件协议、任务发布和阅读；上游由本机协议样例替代。"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import sys
import time
from typing import Any
import urllib.error
import urllib.request
import uuid

sys.dont_write_bytecode = True

from project_env import ROOT
from verify import Client, generate_password
from verify_baseline import Blocked, clean_environment, new_run_root
from verify_collection import CollectionHarness


FIXTURE_GO: str = r'''// 此文件仅由隔离验证脚本生成，不编入应用或发行包。
package main

import (
    "encoding/json"
    "fmt"
    "image"
    "image/color"
    "image/jpeg"
    "io"
    "os"
    "path/filepath"
    "strings"
    "time"
)

// request 保存由真实执行端发来的固定插件协议。
type request struct {
    Action string `json:"action"` // 固定业务动作。
    Query string `json:"query"` // 已验证查询。
    Page int `json:"page"` // 实际页码。
    OrderBy string `json:"orderBy"` // 固定白名单排序，不作为任意上游参数。
    Time string `json:"time"` // 固定白名单时间范围。
    AlbumID string `json:"albumId"` // 已验证编号。
    ChapterIDs []string `json:"chapterIds"` // 明确选择的章节编号，省略或空值表示全本。
    OutputDir string `json:"outputDir"` // 本任务的隔离暂存目录。
    CheckpointDir string `json:"checkpointDir"` // 真实worker分配的私密进度目录。
    Binding string `json:"binding"` // 不可逆的任务与预览摘要，不写入调用日志。
    MaxBytes int64 `json:"maxBytes"` // 总字节预算。
    MaxFiles int `json:"maxFiles"` // 文件数量预算。
    Proxy string `json:"proxy"` // 真实worker分配的公网代理，不在样例中使用网络。
}

// chapterInfo 保存固定协议样例的完整章节目录。
type chapterInfo struct {
    ID string `json:"id"` // 章节正整数编号。
    Title string `json:"title"` // 不含来源地址的中文标题。
    PageCount int `json:"pageCount"` // 完整章节页数。
}

// chapters 返回本地确定性目录，不读取任何真实漫画内容。
func chapters(albumID string) []chapterInfo {
    if albumID == "1010" { return []chapterInfo{{"11","第一章：两页样例",2},{"12","第二章：三页样例",3},{"13","第三章：进度样例",3}} }
    if albumID == "1011" { return []chapterInfo{{"11","可下载的小章节",2},{"12","超预算的大章节",100}} }
    return []chapterInfo{{"11","第一章：隔离样例",2},{"12","第二章：隔离样例",2}}
}

// selected 保留上游顺序，元数据阶段也返回完整目录用于验证执行端选章校验。
func selected(directory []chapterInfo, ids []string) []string {
    result := []string{}
    for _, chapter := range directory {
        include := len(ids) == 0
        for _, id := range ids { if id == chapter.ID { include = true } }
        if include { result = append(result,chapter.ID) }
    }
    return result
}

// progress 在媒体目录之外原子保存四字段页面进度，不包含路径或凭据。
func progress(input request, completed int, total int, bytes int64) {
    if input.CheckpointDir == "" { return }
    if len(input.Binding) != 64 || strings.Trim(input.Binding,"0123456789abcdef") != "" { os.Exit(6) }
    if os.MkdirAll(input.CheckpointDir,0700) != nil { os.Exit(6) }
    body, err := json.Marshal(map[string]any{"completedPages":completed,"totalPages":total,"bytes":bytes,"reusedPages":0})
    if err != nil { os.Exit(6) }
    temporary := filepath.Join(input.CheckpointDir,"progress.json.tmp")
    if os.WriteFile(temporary,body,0600) != nil { os.Exit(6) }
    // Windows读取快照时可能短暂持有不共享删除的句柄；只对本任务原子替换做有界等待。
    for attempt := 0; attempt < 25; attempt++ {
        if err := os.Rename(temporary,filepath.Join(input.CheckpointDir,"progress.json")); err == nil { return } else if !os.IsPermission(err) { os.Exit(6) }
        time.Sleep(20*time.Millisecond)
    }
    os.Exit(6)
}

// main 用固定本地图片模拟桥接协议，不执行传入的Python代码或访问站点。
func main() {
    var input request
    if json.NewDecoder(io.LimitReader(os.Stdin, 65537)).Decode(&input) != nil { os.Exit(2) }
    executable, err := os.Executable()
    if err != nil { os.Exit(2) }
    directory := filepath.Dir(executable)
    log, err := os.OpenFile(filepath.Join(directory, "calls.jsonl"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
    if err != nil { os.Exit(2) }
    _ = json.NewEncoder(log).Encode(map[string]any{"action":input.Action,"albumId":input.AlbumID,"page":input.Page,"orderBy":input.OrderBy,"time":input.Time,"chapterIds":input.ChapterIDs,"maxFiles":input.MaxFiles,"hasCheckpoint":input.CheckpointDir!="","hasBinding":len(input.Binding)==64})
    _ = log.Close()
    var output any
    switch input.Action {
    case "probe":
        output = map[string]any{"version":"2.7.12","protocolVersion":1}
    case "search":
        if input.Proxy == "" { os.Exit(3) }
        if input.Query == "error" { os.Exit(4) }
        if input.Query == "slow" { time.Sleep(3*time.Second) }
        items := []map[string]any{}
        total := 3
        hasNext := input.Page == 1
        if input.Query == "empty" { total = 0; hasNext = false } else if input.Query == "advanced" || input.Query == "1010" || input.Query == "1011" {
            total = 2
            hasNext = false
            items = append(items,map[string]any{"albumId":"1010","title":"高级样例：章节选择与逐页进度","tags":[]string{"高级能力","三章节"}})
            items = append(items,map[string]any{"albumId":"1011","title":"超预算样例：缩小章节后下载","tags":[]string{"只读目录","102页"}})
        } else if input.Page == 1 {
            items = append(items, map[string]any{"albumId":"1001","title":"本地样例漫画：多章顺序与中文标题","tags":[]string{"隔离样例","无真实站点"}})
            items = append(items, map[string]any{"albumId":"1002","title":"第二部样例漫画","tags":[]string{}})
        } else if input.Page == 2 { items = append(items,map[string]any{"albumId":"1003","title":"后续页样例漫画","tags":[]string{"分页"}}) }
        output = map[string]any{"query":input.Query,"page":input.Page,"total":total,"hasNext":hasNext,"items":items}
    case "album", "download":
        if input.Proxy == "" { os.Exit(3) }
        if input.AlbumID == "1999" { os.Exit(4) }
        details := chapters(input.AlbumID)
        if input.AlbumID == "1012" { if _, err := os.Stat(filepath.Join(directory,"metadata-drift")); err == nil { details[0].PageCount = 1; details[1].PageCount = 3 } }
        chapterIDs := []string{}
        pageCount := 0
        for _, chapter := range details { chapterIDs = append(chapterIDs,chapter.ID); pageCount += chapter.PageCount }
        selection := selected(details,input.ChapterIDs)
        selectedPages := 0
        for _, chapter := range details { for _, id := range selection { if id == chapter.ID { selectedPages += chapter.PageCount } } }
        title := "本地样例漫画 " + input.AlbumID
        if input.AlbumID == "1001" { title = "本地样例漫画：多章顺序与中文标题" }
        files := 0
        if input.Action == "download" {
            if input.AlbumID == "1008" { if _, err := os.Stat(filepath.Join(directory,"fail-download")); err == nil { os.Exit(4) } }
            if input.AlbumID == "1009" { time.Sleep(45*time.Second) }
            if len(input.ChapterIDs) > 0 && len(selection) != len(input.ChapterIDs) { os.Exit(4) }
            var totalBytes int64
            progress(input,0,selectedPages,0)
            for chapterIndex, chapter := range details {
                include := false
                for _, id := range selection { if id == chapter.ID { include = true } }
                if !include { continue }
                directory := filepath.Join(input.OutputDir,fmt.Sprintf("%05d",chapterIndex+1))
                if os.MkdirAll(directory,0700) != nil { os.Exit(5) }
                for index := 0; index < chapter.PageCount; index++ {
                    if input.AlbumID == "1006" && files == 3 { break }
                    if input.AlbumID == "1010" { time.Sleep(850*time.Millisecond) }
                    path := filepath.Join(directory,fmt.Sprintf("%05d.jpg",index+1))
                    file, err := os.Create(path)
                    if err != nil { os.Exit(5) }
                    picture := image.NewRGBA(image.Rect(0,0,32,48))
                    for y := 0; y < 48; y++ { for x := 0; x < 32; x++ { picture.SetRGBA(x,y,color.RGBA{R:uint8(40+(chapterIndex*3+index)*20),G:uint8(x*3),B:uint8(y*3),A:255}) } }
                    if jpeg.Encode(file,picture,&jpeg.Options{Quality:90}) != nil || file.Close() != nil { os.Exit(5) }
                    stat, err := os.Stat(path)
                    if err != nil { os.Exit(5) }
                    totalBytes += stat.Size()
                    files++
                    progress(input,files,selectedPages,totalBytes)
                }
            }
            if input.AlbumID == "1007" { if os.WriteFile(filepath.Join(input.OutputDir,"unexpected.txt"),[]byte("not an image"),0600) != nil { os.Exit(5) } }
        }
        output = map[string]any{"albumId":input.AlbumID,"title":title,"pageCount":pageCount,"chapters":chapterIDs,"files":files,"chapterDetails":details,"selectedChapterIds":selection}
    default:
        os.Exit(2)
    }
    if json.NewEncoder(os.Stdout).Encode(output) != nil { os.Exit(2) }
}
'''  # 样例只实现固定JSON协议；它不能证明真实Python桥接或真实站点可用。


class JMComicHarness(CollectionHarness):
    """复用已有隔离媒体、worker、下载兼容样例和进程生命周期。"""

    def __init__(self, args: argparse.Namespace, root: Path) -> None:
        """生成仅本次使用的协议样例工具，并明确开启漫画能力。"""
        self.fixture_dir: Path = root / 'fixture-tool'  # 所有生成源文件和二进制只在隔离目录。
        self.fixture_dir.mkdir()
        source = self.fixture_dir / 'main.go'
        source.write_text(FIXTURE_GO, encoding='utf-8')
        self.fixture_binary: Path = self.fixture_dir / ('jmcomic-fixture.exe' if os.name == 'nt' else 'jmcomic-fixture')  # 固定受信工具配置。
        subprocess.run([sys.executable, '-B', str(ROOT / 'scripts/project_env.py'), '--', 'go', 'build', '-o', str(self.fixture_binary), str(source)], cwd=ROOT, check=True, timeout=180)
        # 编译先于来源线程启动，依赖缺失或构建失败时不会留下后台样例服务。
        super().__init__(args, root)
        self.worker_env.update({
            'NAS_COLLECTION_WORKER_JMCOMIC_ENABLED': 'true',  # 仅本次样例启用固定插件。
            'NAS_COLLECTION_WORKER_JMCOMIC_PYTHON': str(self.fixture_binary),  # 隔离受信解释器协议替身，不修改生产工具。
        })
        self.credentials: dict[str, str] = {'username': 'jmcomic-verify', 'password': generate_password()}  # 本次生成，仅写私有文件供浏览器登录。

    def initialize(self) -> dict[str, Any]:
        """启动真实服务并建立本次专用账号和漫画来源。"""
        self.start_worker()
        self.start()
        self.client.json('/api/setup', 'POST', self.credentials)
        private = self.root / 'app-credentials.json'
        private.write_text(json.dumps(self.credentials), encoding='utf-8')
        if os.name != 'nt':
            private.chmod(0o600)
        return self.source('jmcomic', 'JMComic 隔离漫画样例')

    def worker_request(self, path: str, method: str = 'GET', data: dict[str, Any] | None = None, authenticated: bool = True) -> tuple[int, dict[str, Any]]:
        """直接验证worker认证与固定协议，不回显令牌或原始工具响应。"""
        headers: dict[str, str] = {'Content-Type': 'application/json'}  # worker接收标准JSON。
        if authenticated:
            headers['Authorization'] = 'Bearer ' + self.token_file.read_text(encoding='utf-8').strip()
        body = None if data is None else json.dumps(data).encode()
        request = urllib.request.Request(self.worker_base + path, data=body, headers=headers, method=method)
        try:
            response = urllib.request.urlopen(request, timeout=30)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            return response.status, json.loads(response.read(1024 * 1024))

    def set_source_enabled(self, source: dict[str, Any], enabled: bool) -> dict[str, Any]:
        """使用完整正式提交体切换来源，保留实际修订号。"""
        body: dict[str, Any] = {key: source[key] for key in ('kind', 'name', 'rootId', 'path', 'allowPrivate', 'refreshMinutes', 'outputProfile', 'externalUrl', 'revision')}
        body['enabled'] = enabled
        return self.client.json('/api/collection/sources/' + source['id'], 'PATCH', body)['source']

    def preview(self, source: dict[str, Any], album_id: str, chapter_ids: list[str] | None = None) -> dict[str, Any]:
        """通过应用固定编号参数读取元数据，不直接运行获取。"""
        body: dict[str, Any] = {'sourceId': source['id'], 'albumId': album_id}  # 默认保持旧版省略选章字段的协议。
        if chapter_ids is not None:
            body['chapterIds'] = chapter_ids
        return self.client.json('/api/collection/previews', 'POST', body)['preview']

    def create(self, source: dict[str, Any], album_id: str) -> tuple[dict[str, Any], dict[str, Any]]:
        """先预览后创建实际漫画任务，返回稳定请求体以复核重试。"""
        preview = self.preview(source, album_id)
        body: dict[str, Any] = {'requestId': str(uuid.uuid4()), 'previewId': preview['id'], 'contentType': 'comic', 'createLibrary': True, 'libraryId': 0}
        return self.client.json('/api/collection/tasks', 'POST', body)['task'], body

    def download_count(self, album_id: str) -> int:
        """只读取本次样例的脱敏调用计数，用于验证重复提交不会重复获取。"""
        path = self.fixture_dir / 'calls.jsonl'
        if not path.exists():
            return 0
        calls = [json.loads(line) for line in path.read_text(encoding='utf-8').splitlines() if line]
        return sum(item['action'] == 'download' and item['albumId'] == album_id for item in calls)

    def verify_reader(self, task: dict[str, Any], expected_pages: int = 4) -> None:
        """确认实际漫画聚合项及其原始图片可经过鉴权阅读。"""
        client = self.client
        client.check(task['importState'] == 'imported' and bool(task['itemIds']), '图片下载、发布校验和真实扫描完成后才标记入库')
        pages = 0
        for item_id in task['itemIds']:
            item = client.json(f'/api/items/{item_id}')['item']
            client.check(item['type'] == 'comic', '入库项目保持漫画分类')
            manifest = client.json(f'/api/items/{item_id}/comic')['pages']
            client.check([page['index'] for page in manifest] == list(range(len(manifest))), '漫画页码连续并从零开始')
            for page in manifest:
                status, headers, body = client.request(page['url'])
                client.check(status == 200 and body.startswith(b'\xff\xd8\xff') and headers['Content-Type'].startswith('image/'), '现有漫画阅读API返回实际样例图片')
                client.check(Client(self.base).request(page['url'])[0] == 401, '漫画原图继续要求会话鉴权')
            pages += len(manifest)
        client.check(pages == expected_pages, f'所选{expected_pages}张图片均有实际漫画索引覆盖')

    def run_jmcomic(self) -> None:
        """验证搜索、预览、幂等、失败隔离、取消、重启和原收集兼容。"""
        source = self.initialize()
        client = self.client
        endpoint = '/api/collection/sources/' + source['id'] + '/jmcomic/search'
        client.check(Client(self.base).request(endpoint, 'POST', {'query': 'fixture', 'page': 1})[0] == 401, '漫画搜索复用现有管理员认证')
        client.check(client.request(endpoint, 'POST', {'query': 'fixture', 'page': 1}, {'Origin': 'https://foreign.invalid'})[0] == 403, '漫画搜索拒绝跨站请求')
        client.check(self.worker_request('/v1/jmcomic/search', 'POST', {'query': 'fixture', 'page': 1}, False)[0] == 401, '执行端漫画搜索必须持有专用令牌')
        caps = client.json('/api/collection/capabilities')
        capability = next(item for item in caps['capabilities'] if item['kind'] == 'jmcomic')
        client.check(caps['workerReady'] and capability['available'] and capability['version'] == '2.7.12', '真实worker探测固定版本样例工具并核验共享根')
        client.check(client.json('/api/collection/sources/' + source['id'] + '/test', 'POST', {})['ok'], '漫画来源测试确认真实插件能力及授权')
        first = client.json(endpoint, 'POST', {'query': 'fixture', 'page': 1})
        client.check(first['page'] == 1 and first['total'] == 3 and first['hasNext'] and len(first['items']) == 2, '漫画关键词搜索只返回当前页与明确下一页状态')
        second = client.json(endpoint, 'POST', {'query': 'fixture', 'page': 2})
        client.check(second['page'] == 2 and not second['hasNext'] and len(second['items']) == 1, '漫画搜索第二页保留字符串编号')
        client.check(isinstance(second['items'][0]['albumId'], str) and second['items'][0]['albumId'] == '1003', '漫画编号不会经浮点数或网址传递')
        empty = client.json(endpoint, 'POST', {'query': 'empty', 'page': 1})
        client.check(empty['items'] == [] and empty['total'] == 0, '成功空搜索返回空数组而不是错误或null')
        client.check(client.request(endpoint, 'POST', {'query': 'error', 'page': 1})[0] == 502 and client.request('/api/libraries')[0] == 200, '工具失败返回模块错误并保留应用会话')
        for body in ({'query': '', 'page': 1}, {'query': 'x\ny', 'page': 1}, {'query': 'fixture', 'page': -1}, {'query': 'fixture', 'page': 1001}, {'query': 'fixture', 'page': 1, 'command': 'arbitrary'}):
            client.check(client.request(endpoint, 'POST', body)[0] == 400, '搜索拒绝无效关键词、页码或任意附加参数')
        for album_id in ('0', '01', '-1', '../escape', 'https://invalid.example/1', '1' * 13, '１２'):
            client.check(client.request('/api/collection/previews', 'POST', {'sourceId': source['id'], 'albumId': album_id})[0] == 400, '漫画预览仅接受一至十二位无前导零的正整数编号')
        preview = self.preview(source, '1001')
        client.check(preview['kind'] == 'jmcomic' and preview['profile'] == 'comic' and preview['files'] == [], '漫画预览只读取元数据，固定漫画分类且不宣称已有图片')
        body = {'requestId': str(uuid.uuid4()), 'previewId': preview['id'], 'contentType': 'video', 'createLibrary': True, 'libraryId': 0}
        client.check(client.request('/api/collection/tasks', 'POST', body)[0] == 400, '漫画预览不能改用视频分类绕过入库约束')
        source = self.set_source_enabled(source, False)
        client.check(client.request(endpoint, 'POST', {'query': 'fixture', 'page': 1})[0] == 409, '停用漫画来源后禁止新搜索')
        client.check(client.request('/api/collection/previews', 'POST', {'sourceId': source['id'], 'albumId': '1001'})[0] == 409, '停用漫画来源后禁止新预览')
        body['contentType'] = 'comic'
        client.check(client.request('/api/collection/tasks', 'POST', body)[0] == 409, '过期来源修订的旧预览不能创建任务')
        source = self.set_source_enabled(source, True)
        task, accepted = self.create(source, '1001')
        repeated = client.json('/api/collection/tasks', 'POST', accepted)['task']
        client.check(repeated['id'] == task['id'], '未知提交结果使用原请求编号定位同一持久任务')
        changed = dict(accepted, contentType='music')
        client.check(client.request('/api/collection/tasks', 'POST', changed)[0] in (400, 409), '已绑定请求不能修改内容分类')
        client.check(client.request('/api/collection/tasks', 'POST', dict(accepted, requestId=str(uuid.uuid4())))[0] == 409, '同一资源用新请求提交明确冲突')
        imported = self.wait_collection(task['id'], lambda item: item['importState'] == 'imported' or item['state'] == 'failed')
        self.verify_reader(imported)
        client.check(self.download_count('1001') == 1, '幂等重试只触发一次实际协议获取')
        client.check(bool(client.json('/api/collection/tasks/' + task['id'] + '/events')['events']), '漫画任务使用原有持久中文事件流')
        for album_id in ('1006', '1007'):
            failed, _ = self.create(source, album_id)
            failed = self.wait_collection(failed['id'], lambda item: item['state'] == 'failed')
            client.check(not failed['itemIds'] and failed['importState'] != 'imported' and not (self.worker_root / 'complete' / failed['id']).exists(), '不完整下载或非图片产物被隔离且不能发布入库')
        flag = self.fixture_dir / 'fail-download'
        flag.write_text('controlled failure', encoding='utf-8')
        retry, _ = self.create(source, '1008')
        retry = self.wait_collection(retry['id'], lambda item: item['state'] == 'failed')
        flag.unlink()
        client.json('/api/collection/tasks/' + retry['id'] + '/actions', 'POST', {'action': 'retry', 'revision': retry['revision']})
        retried = self.wait_collection(retry['id'], lambda item: item['importState'] == 'imported' or item['state'] == 'failed')
        client.check(retried['importState'] == 'imported' and self.download_count('1008') == 2, '明确失败任务沿用原操作号重试并成功入库')
        canceled, _ = self.create(source, '1009')
        canceled = self.wait_collection(canceled['id'], lambda item: item['state'] == 'acquiring')
        deadline = time.monotonic() + 10
        while self.download_count('1009') == 0 and time.monotonic() < deadline:
            time.sleep(.1)
        client.check(self.download_count('1009') == 1, '慢速下载实际启动后再验证进程取消')
        canceled = client.json('/api/collection/tasks/' + canceled['id'])['task']
        client.json('/api/collection/tasks/' + canceled['id'] + '/actions', 'POST', {'action': 'cancel', 'revision': canceled['revision']})
        canceled = self.wait_collection(canceled['id'], lambda item: item['state'] == 'canceled', timeout=25)
        client.check(not canceled['itemIds'] and not (self.worker_root / 'complete' / canceled['id']).exists(), '取消慢速获取不会发布暂存图片或产生可读入库项')
        # 原无AlbumID的本地任务仍走既有哈希与发布链路，避免新字段破坏历史协议。
        caps = client.json('/api/collection/capabilities')
        local_root = next(item['id'] for item in caps['roots'] if Path(item['path']).resolve() == (self.root / 'media').resolve())
        local = self.source('local', '原本地登记兼容样例', rootId=local_root, path='music')
        original = self.collect(local)
        with sqlite3.connect(self.root / 'data/library.db') as database:
            client.check(database.execute('PRAGMA user_version').fetchone()[0] == 6 and database.execute('PRAGMA integrity_check').fetchone()[0] == 'ok', '新增插件保持schema 6并通过数据库完整性检查')
        self.stop()
        self.stop_worker()
        self.start_worker()
        self.start()
        client.json('/api/login', 'POST', self.credentials)
        restored = client.json('/api/collection/tasks/' + task['id'])['task']
        client.check(restored['itemIds'] == imported['itemIds'] and restored['importState'] == 'imported', '主服务与worker重启后漫画发布和入库关联保留')
        client.check(client.json('/api/collection/tasks', 'POST', accepted)['task']['id'] == task['id'] and self.download_count('1001') == 1, '重启后重试已受理请求不会再次下载漫画')
        client.check(client.json('/api/collection/tasks/' + original['id'])['task']['importState'] == 'imported', '旧本地来源入库任务在插件加入及重启后仍有效')
        self.stop_worker()
        self.worker_env['NAS_COLLECTION_WORKER_JMCOMIC_ENABLED'] = 'false'
        self.start_worker()
        disabled = client.json('/api/collection/capabilities')
        client.check(disabled['workerReady'] and not next(item for item in disabled['capabilities'] if item['kind'] == 'jmcomic')['available'], '关闭可选插件只禁用漫画能力，不影响执行端')
        client.check(client.request(endpoint, 'POST', {'query': 'fixture', 'page': 1})[0] == 503, '插件禁用时搜索明确提示不可用')
        client.check(client.request(f'/api/items/{restored["itemIds"][0]}/comic')[0] == 200, '插件禁用后已发布漫画仍可阅读')
        self.stop_worker()
        self.worker_env['NAS_COLLECTION_WORKER_JMCOMIC_ENABLED'] = 'true'
        self.worker_env['NAS_COLLECTION_WORKER_JMCOMIC_PYTHON'] = str(self.fixture_dir / 'missing-python.exe')
        self.start_worker()
        missing = client.json('/api/collection/capabilities')
        client.check(missing['workerReady'] and not next(item for item in missing['capabilities'] if item['kind'] == 'jmcomic')['available'], '缺少可选Python依赖不阻止基础服务启动')
        self.stop_worker()
        unavailable = client.json('/api/collection/capabilities')
        client.check(not unavailable['workerReady'] and bool(unavailable['workerReason']), '执行端离线返回清晰不可用状态')
        client.check(client.request(f'/api/items/{restored["itemIds"][0]}/comic')[0] == 200, '执行端离线也保留已发布漫画的阅读能力')
        print(f'漫画协议集成验证通过：{client.passed}项；真实应用与worker，固定上游协议样例，未执行真实站点下载。', flush=True)

    def serve_jmcomic(self) -> None:
        """保留隔离服务供浏览器检查；账号存私有文件，不打印凭据。"""
        source = self.initialize()
        info: dict[str, str] = {'base': self.base, 'root': str(self.root), 'sourceId': source['id'], 'credentialFile': str(self.root / 'app-credentials.json'), 'stopFile': str(self.root / 'stop')}
        (self.root / 'browser.json').write_text(json.dumps(info, ensure_ascii=False), encoding='utf-8')
        print(json.dumps(info, ensure_ascii=False), flush=True)
        while not (self.root / 'stop').exists():
            time.sleep(.5)


def main(harness_type: type[JMComicHarness] = JMComicHarness, root_prefix: str = 'jmcomic') -> None:
    """解析明确的隔离工具路径，只创建仓库verify下的全新安全目录。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True, help='提前构建的真实主服务')
    parser.add_argument('--worker-binary', type=Path, required=True, help='提前构建的真实资源获取执行端')
    parser.add_argument('--root', type=Path, help='必须为仓库.runtime/verify下不存在且无链接的隔离根目录')
    parser.add_argument('--serve', action='store_true', help='保留隔离浏览器服务，创建根目录stop文件后关闭')
    parser.set_defaults(rclone=None, ytdlp=None, ffmpeg=None, web_url='')
    args = parser.parse_args()
    for binary in (args.binary, args.worker_binary):
        if not binary.is_file():
            parser.error('应用或执行端不存在，请先构建')
    try:
        root = new_run_root(str(args.root) if args.root is not None else None, root_prefix)
    except Blocked as error:
        parser.error(str(error))
    # 继承构建缓存约束，但不让用户实例配置、代理或外部连接进入本次样例。
    environment = clean_environment()
    os.environ.clear()
    os.environ.update(environment)
    harness = harness_type(args, root)
    passed = False
    try:
        if args.serve:
            harness.serve_jmcomic()
        else:
            harness.run_jmcomic()
            passed = True
    finally:
        try:
            harness.close()
        except BaseException:
            passed = False
            raise
        finally:
            if not args.serve:
                result = {'passed': passed, 'assertions': harness.client.passed, 'realApplication': True, 'realWorker': True, 'pythonBridgeExecuted': False, 'upstreamProtocolFixture': True, 'realSiteSearch': False, 'realSiteDownload': False, 'dockerExecuted': False}
                (root / 'result.json').write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding='utf-8')


if __name__ == '__main__':
    from project_env import run_cli

    raise SystemExit(run_cli(main))
