"""用真实应用和worker核验UAV收集闭环；站点及下载器由明确的离线协议样例代替。"""

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

from project_env import ROOT, run_cli
from verify import Client, generate_password
from verify_baseline import Blocked, clean_environment, new_run_root
from verify_collection import CollectionHarness


FIXTURE_GO: str = r'''// 此工具只在本次隔离验证中编译，不进入发行包或生产配置。
package main

import (
    "encoding/json"
    "io"
    "net/url"
    "os"
    "path"
    "path/filepath"
    "strings"
    "time"
)

// request 接收正式桥接的有界输入，不执行传入Python脚本。
type request struct {
    Action string `json:"action"` // 固定动作。
    Site string `json:"site"` // 固定站点。
    Query string `json:"query"` // 已验证关键词。
    Page int `json:"page"` // 已验证页码。
    VideoID string `json:"videoId"` // 规范视频编号。
    SourceOffset int `json:"sourceOffset"` // 仅私有SupJav解析使用的线路起点。
    ID string `json:"id"` // 固定下载意图携带的规范视频编号。
    URL string `json:"url"` // 仅下载器协议的私有媒体地址。
}

// record 只保存样例动作和编号，不保存网址、令牌或搜索词。
func record(directory string, action string, id string) {
    file, err := os.OpenFile(filepath.Join(directory,"calls.jsonl"),os.O_APPEND|os.O_CREATE|os.O_WRONLY,0600)
    if err != nil { os.Exit(2) }
    _ = json.NewEncoder(file).Encode(map[string]string{"action":action,"videoId":id})
    _ = file.Close()
}

// main 同时模拟固定桥接和下载器协议，实际视频及后处理仍由真实FFmpeg生成与校验。
func main() {
    executable, err := os.Executable()
    if err != nil { os.Exit(2) }
    directory := filepath.Dir(executable)
    downloader := false
    outputPath := ""
    for index, value := range os.Args[1:] {
        if value == "--version" { _, _ = os.Stdout.WriteString("2026.11.25\n"); return }
        if value == "--load-info-json" { downloader = true }
        if (value == "--output" || value == "-o") && index+2 < len(os.Args) { outputPath = os.Args[index+2] }
    }
    var input request
    if json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&input) != nil { os.Exit(2) }
    if downloader {
        if os.Getenv("HTTPS_PROXY") == "" || outputPath == "" { os.Exit(3) }
        media, err := url.Parse(input.URL)
        if err != nil { os.Exit(2) }
        id := input.ID
        if id == "" { id = strings.TrimSuffix(path.Base(media.Path),".mp4") }
        record(directory,"download",id)
        if id == "1105" { os.Exit(4) }
        if id == "1104" {
            flag := filepath.Join(directory,"supjav-first-download-failed")
            if _, err := os.Stat(flag); os.IsNotExist(err) {
                if os.WriteFile(flag,[]byte("controlled failure"),0600) != nil { os.Exit(5) }
                os.Exit(4)
            }
        }
        if id == "1008" { if _, err := os.Stat(filepath.Join(directory,"fail-download")); err == nil { os.Exit(4) } }
        if id == "1009" { time.Sleep(45*time.Second) }
        outputPath = strings.ReplaceAll(outputPath,"%(ext)s","mp4")
        if id == "1007" { return }
        if id == "1006" { if os.WriteFile(outputPath,[]byte("invalid video"),0600) != nil { os.Exit(5) }; return }
        source, err := os.Open(filepath.Join(directory,"fixture.mp4"))
        if err != nil { os.Exit(5) }
        defer source.Close()
        target, err := os.Create(outputPath)
        if err != nil { os.Exit(5) }
        if _, err := io.Copy(target,source); err != nil { os.Exit(5) }
        if target.Close() != nil { os.Exit(5) }
        return
    }
    record(directory,input.Action,input.VideoID)
    var output any
    switch input.Action {
    case "probe":
        output = map[string]any{"version":"3.8.4","commit":"d40f09457fbd38dd7dc94580637242afafaf3939","protocolVersion":1}
    case "search":
        if os.Getenv("HTTPS_PROXY") == "" { os.Exit(3) }
        if input.Query == "error" { os.Exit(4) }
        if input.Query == "slow" { time.Sleep(3*time.Second) }
        items := []map[string]any{}
        if input.Query != "empty" && input.Page == 1 {
            items = append(items,map[string]any{"videoId":"1001","title":"本地样例视频：搜索下载与中文标题","duration":"00:02"})
            items = append(items,map[string]any{"videoId":"1002","title":"第二部样例视频","duration":""})
        } else if input.Query != "empty" && input.Page == 2 { items = append(items,map[string]any{"videoId":"1003","title":"后续页样例视频","duration":"00:02"}) }
        if input.Site == "huangguo" { for _, item := range items { item["videoId"] = "huangguo-42-ep-" + map[string]string{"1001":"1","1002":"2","1003":"3"}[item["videoId"].(string)] } }
        if input.Site == "supjav" { for _, item := range items { item["videoId"] = map[string]string{"1001":"1101","1002":"1102","1003":"1103"}[item["videoId"].(string)] } }
        output = map[string]any{"site":input.Site,"query":input.Query,"page":input.Page,"hasNext":input.Query!="empty"&&input.Page==1,"items":items}
    case "resolve":
        if os.Getenv("HTTPS_PROXY") == "" { os.Exit(3) }
        if input.VideoID == "1999" { os.Exit(4) }
        output = map[string]any{"site":input.Site,"videoId":input.VideoID,"title":"本地样例视频 "+input.VideoID,"mediaUrl":"https://example.com/uav/"+input.VideoID+".mp4","headers":map[string]string{},"manifest":""}
        if input.VideoID == "1010" { output.(map[string]any)["title"] = strings.Repeat("中",200) }
        if input.Site == "huangguo" {
            output.(map[string]any)["mediaUrl"] = "https://example.com/uav/fixture.m3u8"
            output.(map[string]any)["manifest"] = "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:2,\nhttps://example.com/uav/segment.ts\n#EXT-X-ENDLIST\n"
        }
        if input.Site == "supjav" {
            if input.SourceOffset < 0 || input.SourceOffset > 6 { os.Exit(2) }
            output.(map[string]any)["sourceIndex"] = 6
            if (input.VideoID == "1104" || input.VideoID == "1105") && input.SourceOffset == 0 {
                output.(map[string]any)["sourceIndex"] = 0
                output.(map[string]any)["mediaUrl"] = "https://example.com/uav/fixture.m3u8"
                output.(map[string]any)["manifest"] = "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:2,\nhttps://example.com/uav/segment.ts\n#EXT-X-ENDLIST\n"
            }
        }
    default:
        os.Exit(2)
    }
    if json.NewEncoder(os.Stdout).Encode(output) != nil { os.Exit(2) }
}
'''  # 仅验证正式协议和状态机，不能证明真实Python、yt-dlp或公网站点工作正常。


class UAVHarness(CollectionHarness):
    """管理真实收集服务、离线固定工具和真实可解码视频。"""

    def __init__(self, args: argparse.Namespace, root: Path) -> None:
        """先生成受控工具和MP4，再启动任何来源线程。"""
        self.fixture_dir: Path = root / 'fixture-tool'  # 本次所有生成源码和二进制的隔离位置。
        self.fixture_dir.mkdir()
        source = self.fixture_dir / 'main.go'
        source.write_text(FIXTURE_GO, encoding='utf-8')
        self.fixture_binary: Path = self.fixture_dir / ('uav-fixture.exe' if os.name == 'nt' else 'uav-fixture')  # 固定受信协议样例。
        subprocess.run([sys.executable, '-B', str(ROOT / 'scripts/project_env.py'), '--', 'go', 'build', '-o', str(self.fixture_binary), str(source)], cwd=ROOT, check=True, timeout=180)
        subprocess.run([str(args.ffmpeg.resolve()), '-hide_banner', '-loglevel', 'error', '-nostdin', '-f', 'lavfi', '-i', 'testsrc2=size=160x90:rate=12', '-t', '2', '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', str(self.fixture_dir / 'fixture.mp4')], check=True, timeout=30)
        args.ytdlp = self.fixture_binary
        super().__init__(args, root)
        self.worker_env.update({
            'NAS_COLLECTION_WORKER_UAV_ENABLED': 'true',  # 显式开启本次固定视频样例。
            'NAS_COLLECTION_WORKER_UAV_PYTHON': str(self.fixture_binary),  # 受信协议替身，不执行Python或联网。
        })
        self.credentials: dict[str, str] = {'username': 'uav-verify', 'password': generate_password()}  # 仅保存到本次私有文件供浏览器登录。

    def initialize(self) -> dict[str, Any]:
        """启动真实服务，创建本次专用用户和视频来源。"""
        self.start_worker()
        self.start()
        self.client.json('/api/setup', 'POST', self.credentials)
        private = self.root / 'app-credentials.json'
        private.write_text(json.dumps(self.credentials), encoding='utf-8')
        if os.name != 'nt':
            private.chmod(0o600)
        return self.source('uav', 'UAV 隔离视频样例')

    def worker_request(self, path: str, method: str = 'GET', data: dict[str, Any] | None = None, authenticated: bool = True) -> tuple[int, dict[str, Any]]:
        """直接检查worker令牌认证，不回显认证或原始工具输出。"""
        headers: dict[str, str] = {'Content-Type': 'application/json'}  # 固定JSON协议。
        if authenticated:
            headers['Authorization'] = 'Bearer ' + self.token_file.read_text(encoding='utf-8').strip()
        request = urllib.request.Request(self.worker_base + path, data=None if data is None else json.dumps(data).encode(), headers=headers, method=method)
        try:
            response = urllib.request.urlopen(request, timeout=30)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            return response.status, json.loads(response.read(1 << 20))

    def set_source_enabled(self, source: dict[str, Any], enabled: bool) -> dict[str, Any]:
        """经正式完整表单更新来源，保留实际修订号。"""
        body: dict[str, Any] = {key: source[key] for key in ('kind', 'name', 'rootId', 'path', 'allowPrivate', 'refreshMinutes', 'outputProfile', 'externalUrl', 'revision')}
        body['enabled'] = enabled
        return self.client.json('/api/collection/sources/' + source['id'], 'PATCH', body)['source']

    def preview(self, source: dict[str, Any], video_id: str, site: str = 'jable') -> dict[str, Any]:
        """只读取选定站点和编号的元数据，不启动下载器。"""
        return self.client.json('/api/collection/previews', 'POST', {'sourceId': source['id'], 'uavSite': site, 'videoId': video_id})['preview']

    def create(self, source: dict[str, Any], video_id: str, site: str = 'jable') -> tuple[dict[str, Any], dict[str, Any]]:
        """先取有效预览再以稳定UUID创建视频任务。"""
        preview = self.preview(source, video_id, site)
        body: dict[str, Any] = {'requestId': str(uuid.uuid4()), 'previewId': preview['id'], 'contentType': 'video', 'createLibrary': True, 'libraryId': 0}
        return self.client.json('/api/collection/tasks', 'POST', body)['task'], body

    def download_count(self, video_id: str) -> int:
        """读取本次样例下载调用数，核验幂等和取消是否真正触发。"""
        file = self.fixture_dir / 'calls.jsonl'
        calls: list[dict[str, str]] = [json.loads(line) for line in file.read_text(encoding='utf-8').splitlines() if line] if file.exists() else []
        return sum(item['action'] == 'download' and item['videoId'] == video_id for item in calls)

    def verify_video(self, task: dict[str, Any]) -> None:
        """确认真实视频产物入库，继续使用鉴权Range读取。"""
        client = self.client
        client.check(task['importState'] == 'imported' and len(task['itemIds']) == 1, '实际视频校验、发布和扫描后才关联一个入库项目')
        item_id = task['itemIds'][0]
        client.check(client.json(f'/api/items/{item_id}')['item']['type'] == 'video', 'UAV产物固定导入视频库')
        status, headers, body = client.request(f'/api/items/{item_id}/file', headers={'Range': 'bytes=0-31'})
        client.check(status == 206 and len(body) == 32 and b'ftyp' in body and headers['Content-Range'].startswith('bytes 0-31/'), '真实后处理MP4通过现有Range播放接口读取')
        client.check(Client(self.base).request(f'/api/items/{item_id}/file')[0] == 401, '已发布视频继续要求会话鉴权')

    def run_uav(self) -> None:
        """覆盖五站搜索、黄果单集、SupJav导入、严格参数和持久任务恢复。"""
        source = self.initialize()
        client = self.client
        endpoint = '/api/collection/sources/' + source['id'] + '/uav/search'
        search: dict[str, Any] = {'site': 'jable', 'query': 'fixture', 'page': 1}  # 固定有界样例请求。
        client.check(Client(self.base).request(endpoint, 'POST', search)[0] == 401, '视频搜索复用管理员会话鉴权')
        client.check(client.request(endpoint, 'POST', search, {'Origin': 'https://foreign.invalid'})[0] == 403, '视频搜索拒绝跨站请求')
        client.check(self.worker_request('/v1/uav/search', 'POST', search, False)[0] == 401, 'worker视频搜索必须持有专用令牌')
        caps = client.json('/api/collection/capabilities')
        capability = next(item for item in caps['capabilities'] if item['kind'] == 'uav')
        client.check(caps['workerReady'] and capability['available'] and capability['version'] == '3.8.4', '真实worker确认固定桥接版本、下载依赖与共享根')
        client.check(client.json('/api/collection/sources/' + source['id'] + '/test', 'POST', {})['ok'], 'UAV来源测试复用真实插件能力探测')
        for site in ('jable', 'missav', 'hanime1', 'huangguo', 'supjav'):
            first = client.json(endpoint, 'POST', dict(search, site=site))
            client.check(first['site'] == site and first['page'] == 1 and first['hasNext'] and len(first['items']) == 2 and 'total' not in first, site + '搜索只返回当前页和下一页提示，不编造总数')
            if site == 'huangguo':
                client.check([item['videoId'] for item in first['items']] == ['huangguo-42-ep-1', 'huangguo-42-ep-2'], '黄果搜索以作品和集号组成的规范单集编号选择')
                later = client.json(endpoint, 'POST', dict(search, site=site, page=2))
                client.check(not later['hasNext'] and later['items'][0]['videoId'] == 'huangguo-42-ep-3', '黄果后续页保留实际单集编号与分页语义')
            if site == 'supjav':
                later = client.json(endpoint, 'POST', dict(search, site=site, page=2))
                client.check(first['items'][0]['videoId'] == '1101' and not later['hasNext'] and later['items'][0]['videoId'] == '1103', 'SupJav搜索保留规范视频编号和真实分页契约')
                client.check(client.json(endpoint, 'POST', dict(search, site=site, query='empty'))['items'] == [], 'SupJav空搜索返回空数组')
                client.check(client.request(endpoint, 'POST', dict(search, site=site, query='error'))[0] == 502, 'SupJav站点失败不伪装成空结果')
        second = client.json(endpoint, 'POST', dict(search, page=2))
        client.check(not second['hasNext'] and second['items'][0]['videoId'] == '1003' and isinstance(second['items'][0]['duration'], str), '第二页保持视频编号和时长的字符串类型')
        client.check(client.json(endpoint, 'POST', dict(search, query='empty'))['items'] == [], '成功空搜索返回空数组')
        client.check(client.request(endpoint, 'POST', dict(search, query='error'))[0] == 502 and client.request('/api/libraries')[0] == 200, '站点错误明确失败且应用会话保持有效')
        for invalid in (dict(search, site='unknown'), dict(search, query=''), dict(search, query='a\nb'), dict(search, query='字' * 121), dict(search, page=-1), dict(search, page=1001), dict(search, command='arbitrary')):
            client.check(client.request(endpoint, 'POST', invalid)[0] == 400, '搜索拒绝不支持站点、无效关键词页码或附加命令')
        for video_id in ('../escape', 'https://example.com/video', 'x/y', 'a' * 201, '１２'):
            client.check(client.request('/api/collection/previews', 'POST', {'sourceId': source['id'], 'uavSite': 'jable', 'videoId': video_id})[0] == 400, '预览编号不能携带URL路径或非规范文本')
        for video_id in ('42', 'huangguo-42', 'huangguo-42-ep-0', 'huangguo-042-ep-1', 'huangguo-42-ep-01', 'huangguo-9223372036854775808-ep-1', 'huangguo-42-ep-2147483648'):
            client.check(client.request('/api/collection/previews', 'POST', {'sourceId': source['id'], 'uavSite': 'huangguo', 'videoId': video_id})[0] == 400, '黄果拒绝整剧、缺集号、前导零及越界编号')
        for video_id in ('0', '01', '-1', 'abc123', '1101.html', 'https://supjav.com/1101.html', '１２'):
            client.check(client.request('/api/collection/previews', 'POST', {'sourceId': source['id'], 'uavSite': 'supjav', 'videoId': video_id})[0] == 400, 'SupJav拒绝非规范数字编号和网址输入')
        for extra in ({'url': 'https://example.com'}, {'rootId': 'fixture-local'}, {'allowPrivate': True}, {'externalUrl': 'https://example.com'}, {'path': 'escape'}):
            body: dict[str, Any] = {'kind': 'uav', 'name': '拒绝无关配置', 'enabled': True, 'rootId': '', 'path': '', 'allowPrivate': False, 'refreshMinutes': 30, 'outputProfile': 'video', 'externalUrl': ''}
            body.update(extra)
            client.check(client.request('/api/collection/sources', 'POST', body)[0] == 400, '固定插件来源拒绝网址、私网权限和目录等隐藏参数')
        preview = self.preview(source, '1001')
        client.check(preview['kind'] == 'uav' and preview['profile'] == 'video' and preview['files'] == [] and self.download_count('1001') == 0, '预览固定视频类型，只取元数据且不下载媒体')
        client.check(self.preview(source, '1010')['title'] == '中' * 200, '合法长中文视频标题在搜索与预览边界保持兼容')
        huangguo_preview = self.preview(source, 'huangguo-42-ep-1', 'huangguo')
        client.check(huangguo_preview['files'] == [] and self.download_count('huangguo-42-ep-1') == 0 and 'mediaUrl' not in huangguo_preview and 'cdn' not in huangguo_preview, '黄果预览不获取媒体，不向浏览器暴露CDN资料')
        body = {'requestId': str(uuid.uuid4()), 'previewId': preview['id'], 'contentType': 'comic', 'createLibrary': True, 'libraryId': 0}
        client.check(client.request('/api/collection/tasks', 'POST', body)[0] == 400, 'UAV预览不能改用非视频类型')
        source = self.set_source_enabled(source, False)
        client.check(client.request(endpoint, 'POST', search)[0] == 409, '来源停用禁止新搜索')
        client.check(client.request('/api/collection/previews', 'POST', {'sourceId': source['id'], 'uavSite': 'jable', 'videoId': '1001'})[0] == 409, '来源停用禁止新预览')
        client.check(client.request('/api/collection/tasks', 'POST', dict(body, contentType='video'))[0] == 409, '来源修订后的旧预览失效')
        source = self.set_source_enabled(source, True)
        task, accepted = self.create(source, '1001')
        client.check(client.json('/api/collection/tasks', 'POST', accepted)['task']['id'] == task['id'], '未知提交结果复用原UUID定位同一持久任务')
        client.check(client.request('/api/collection/tasks', 'POST', dict(accepted, requestId=str(uuid.uuid4())))[0] == 409, '同一视频的新请求明确冲突')
        imported = self.wait_collection(task['id'], lambda item: item['importState'] == 'imported' or item['state'] == 'failed')
        self.verify_video(imported)
        client.check(self.download_count('1001') == 1, '重复提交只启动一次下载器')
        client.check(bool(client.json('/api/collection/tasks/' + task['id'] + '/events')['events']), 'UAV沿用原任务中文持久事件流')
        huangguo_task, _ = self.create(source, 'huangguo-42-ep-1', 'huangguo')
        huangguo_imported = self.wait_collection(huangguo_task['id'], lambda item: item['importState'] == 'imported' or item['state'] == 'failed')
        self.verify_video(huangguo_imported)
        client.check(self.download_count('huangguo-42-ep-1') == 1, '黄果任务只下载选择的单集，复用原发布入库门禁')
        supjav_preview = self.preview(source, '1101', 'supjav')
        client.check(supjav_preview['files'] == [] and self.download_count('1101') == 0 and 'mediaUrl' not in supjav_preview and 'headers' not in supjav_preview, 'SupJav预览仅含公开元数据且不下载媒体')
        supjav_task, supjav_accepted = self.create(source, '1101', 'supjav')
        client.check(client.json('/api/collection/tasks', 'POST', supjav_accepted)['task']['id'] == supjav_task['id'], 'SupJav重复提交定位同一持久任务')
        supjav_imported = self.wait_collection(supjav_task['id'], lambda item: item['importState'] == 'imported' or item['state'] == 'failed')
        self.verify_video(supjav_imported)
        client.check(self.download_count('1101') == 1, 'SupJav只获取所选视频并完成原发布扫描流程')
        fallback, _ = self.create(source, '1104', 'supjav')
        fallback = self.wait_collection(fallback['id'], lambda item: item['importState'] == 'imported' or item['state'] == 'failed')
        self.verify_video(fallback)
        client.check(self.download_count('1104') == 2, 'SupJav首线路获取失败后清理旧产物并改用后续线路')
        exhausted, _ = self.create(source, '1105', 'supjav')
        exhausted = self.wait_collection(exhausted['id'], lambda item: item['state'] == 'failed')
        client.check(self.download_count('1105') == 2 and not exhausted['itemIds'] and not (self.worker_root / 'complete' / exhausted['id']).exists(), 'SupJav全部候选线路获取失败时不发布或入库')
        for video_id in ('1006', '1007'):
            failed, _ = self.create(source, video_id)
            failed = self.wait_collection(failed['id'], lambda item: item['state'] == 'failed')
            client.check(not failed['itemIds'] and not (self.worker_root / 'complete' / failed['id']).exists(), '损坏或缺失视频不能通过后处理及发布门禁')
        flag = self.fixture_dir / 'fail-download'
        flag.write_text('controlled failure', encoding='utf-8')
        retry, _ = self.create(source, '1008')
        retry = self.wait_collection(retry['id'], lambda item: item['state'] == 'failed')
        flag.unlink()
        client.json('/api/collection/tasks/' + retry['id'] + '/actions', 'POST', {'action': 'retry', 'revision': retry['revision']})
        retried = self.wait_collection(retry['id'], lambda item: item['importState'] == 'imported' or item['state'] == 'failed')
        client.check(retried['importState'] == 'imported' and self.download_count('1008') == 2, '明确失败任务沿用原操作重试并完成入库')
        canceled, _ = self.create(source, '1009')
        deadline = time.monotonic() + 15
        while self.download_count('1009') == 0 and time.monotonic() < deadline:
            time.sleep(.1)
        client.check(self.download_count('1009') == 1, '慢速下载真正启动后再验证取消')
        canceled = client.json('/api/collection/tasks/' + canceled['id'])['task']
        client.json('/api/collection/tasks/' + canceled['id'] + '/actions', 'POST', {'action': 'cancel', 'revision': canceled['revision']})
        canceled = self.wait_collection(canceled['id'], lambda item: item['state'] == 'canceled', timeout=25)
        client.check(not canceled['itemIds'] and not (self.worker_root / 'complete' / canceled['id']).exists(), '取消中的视频不会发布或入库')
        local_root = next(item['id'] for item in caps['roots'] if Path(item['path']).resolve() == (self.root / 'media').resolve())
        original = self.collect(self.source('local', '原本地收集兼容样例', rootId=local_root, path='music'))
        with sqlite3.connect(self.root / 'data/library.db') as database:
            client.check(database.execute('PRAGMA user_version').fetchone()[0] == 6 and database.execute('PRAGMA integrity_check').fetchone()[0] == 'ok', 'UAV保持schema 6且数据库完整')
        self.stop()
        self.stop_worker()
        self.start_worker()
        self.start()
        client.json('/api/login', 'POST', self.credentials)
        restored = client.json('/api/collection/tasks/' + task['id'])['task']
        client.check(restored['itemIds'] == imported['itemIds'] and restored['importState'] == 'imported', '主服务与worker重启后视频任务关联仍保留')
        client.check(client.json('/api/collection/tasks', 'POST', accepted)['task']['id'] == task['id'] and self.download_count('1001') == 1, '重启后原请求不会再次下载')
        client.check(client.json('/api/collection/tasks', 'POST', supjav_accepted)['task']['id'] == supjav_task['id'] and self.download_count('1101') == 1 and client.json('/api/collection/tasks/' + supjav_task['id'])['task']['itemIds'] == supjav_imported['itemIds'], 'SupJav重启后保留原任务和入库关联且不重复获取')
        client.check(client.json('/api/collection/tasks/' + original['id'])['task']['importState'] == 'imported', '旧本地收集任务重启后保持兼容')
        self.stop_worker()
        self.worker_env['NAS_COLLECTION_WORKER_UAV_ENABLED'] = 'false'
        self.start_worker()
        disabled = client.json('/api/collection/capabilities')
        client.check(disabled['workerReady'] and not next(item for item in disabled['capabilities'] if item['kind'] == 'uav')['available'], '关闭插件仅禁用UAV能力')
        client.check(client.request(endpoint, 'POST', search)[0] == 503, '禁用插件时搜索明确不可用')
        self.stop_worker()
        self.worker_env['NAS_COLLECTION_WORKER_UAV_ENABLED'] = 'true'
        self.worker_env['NAS_COLLECTION_WORKER_UAV_PYTHON'] = str(self.fixture_dir / 'missing-python.exe')
        self.start_worker()
        missing = client.json('/api/collection/capabilities')
        client.check(missing['workerReady'] and not next(item for item in missing['capabilities'] if item['kind'] == 'uav')['available'], '缺可选解释器不妨碍worker基础启动')
        self.stop_worker()
        unavailable = client.json('/api/collection/capabilities')
        client.check(not unavailable['workerReady'] and bool(unavailable['workerReason']), '执行端离线有明确原因')
        client.check(client.request(f'/api/items/{restored["itemIds"][0]}/file', 'HEAD')[0] == 200, '插件关闭或执行端离线仍可播放已发布视频')
        print(f'UAV协议集成验证通过：{client.passed}项；真实应用、worker及FFmpeg，桥接与下载器为离线协议样例。', flush=True)

    def serve_uav(self) -> None:
        """保留本次自有实例供浏览器验收，凭据不打印。"""
        source = self.initialize()
        info: dict[str, str] = {'base': self.base, 'root': str(self.root), 'sourceId': source['id'], 'credentialFile': str(self.root / 'app-credentials.json'), 'stopFile': str(self.root / 'stop')}
        (self.root / 'browser.json').write_text(json.dumps(info, ensure_ascii=False), encoding='utf-8')
        print(json.dumps(info, ensure_ascii=False), flush=True)
        while not (self.root / 'stop').exists():
            time.sleep(.5)


def main() -> None:
    """只创建.runtime下全新隔离根，所有子进程继承仓库缓存约束。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True, help='提前构建的真实主服务')
    parser.add_argument('--worker-binary', type=Path, required=True, help='提前构建的真实worker')
    parser.add_argument('--ffmpeg', type=Path, required=True, help='用于生成、离线重封装和解码校验的真实FFmpeg')
    parser.add_argument('--root', type=Path, help='必须为仓库.runtime/verify下不存在的隔离根')
    parser.add_argument('--serve', action='store_true', help='保留自有浏览器实例直到根目录出现stop文件')
    parser.set_defaults(rclone=None, ytdlp=None, web_url='')
    args = parser.parse_args()
    if any(not binary.is_file() for binary in (args.binary, args.worker_binary, args.ffmpeg)):
        parser.error('请提供已经构建的应用、worker和FFmpeg')
    try:
        root = new_run_root(str(args.root) if args.root is not None else None, 'uav')
    except Blocked as error:
        parser.error(str(error))
    environment = clean_environment()
    os.environ.clear()
    os.environ.update(environment)
    harness = UAVHarness(args, root)
    passed = False
    try:
        if args.serve:
            harness.serve_uav()
        else:
            harness.run_uav()
            passed = True
    finally:
        harness.close()
        if not args.serve:
            result: dict[str, Any] = {'passed': passed, 'assertions': harness.client.passed, 'sites': ['jable', 'missav', 'hanime1', 'huangguo', 'supjav'], 'realApplication': True, 'realWorker': True, 'realFFmpeg': True, 'pythonBridgeExecuted': False, 'realYtdlpExecuted': False, 'upstreamProtocolFixture': True, 'relayRegistrationExecuted': True, 'realRelayTransferExecuted': False, 'realSiteSearch': False, 'realSiteDownload': False, 'dockerExecuted': False}
            (root / 'result.json').write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding='utf-8')


if __name__ == '__main__':
    raise SystemExit(run_cli(main))
