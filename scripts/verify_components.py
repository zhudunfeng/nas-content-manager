#!/usr/bin/env python3
"""用真实应用和私有 Unix helper 协议夹具验证资源组件部署，不运行 Docker。"""

from __future__ import annotations

import argparse
import copy
import http.server
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import threading
import time
from typing import Any
import urllib.parse
import uuid

from project_env import verification_directory
from verify import Client, generate_password


# 固定协议版本，与生产 DTO 保持一致，不允许夹具接收任意镜像或命令。
TEMPLATE_VERSION: str = 'collection-2026-11-20.1'
# 目录访问授权版本，只有明确确认才能创建任务。
NOTICE_VERSION: str = 'collection-storage-v1'
# 四个固定组件的显示名称、固定版本、宿主配置端口和容器 API 端口。
COMPONENTS: dict[str, tuple[str, str, int, int]] = {
    'moviepilot': ('MoviePilot', '3.2.8', 19130, 3000),
    'autobangumi': ('AutoBangumi', '4.0.0', 19132, 7892),
    'sonarr': ('Sonarr', '4.0.20.3014', 19133, 8989),
    'radarr': ('Radarr', '7.0.3.10918', 19134, 7878),
}
# 浏览器验收控制文件的参数说明，仅作用于本次夹具而非产品配置。
CONTROL_HELP: dict[str, str] = {
    'helper': '助手状态：ready、unavailable、unsupported 或 not_enabled，默认 ready。',
    'hold': '为 true 时保持已受理任务运行，不进入完成或失败，默认 false。',
    'fail': '为 true 时所有到达完成时点的活动任务均失败，默认 false。',
    'failNextToken': '每个新字符串仅让下一次新部署或启动任务失败一次，适合显式重试验收。',
    'dropDeployToken': '每个新字符串仅让下一次已受理部署丢失回执一次；复用原 UUID 可找回同一任务。',
    'stepSeconds': '任务进入完成状态前的最小秒数，限制在 0.1 至 30，默认 0.35。',
    'resetToken': '每个新非空字符串重置四组件、任务和 UUID 绑定；仅用于隔离浏览器场景重置。',
    'delayStatusSeconds': '真实只读状态回执延迟秒数，限制在 0 至 20，默认 0。',
    'goneTasks': '指定任务标识数组读取时返回 404，模拟已过保留期的恢复记录。',
    'malformedTask': '为 true 时任务回执添加未知字段，验证生产严格 DTO 边界，默认 false。',
    'components': '按四个固定组件覆盖 presence、runtime、readiness、message、revision、canDeploy、canStart、serviceUrl、latestTaskId；不接受镜像、命令或任意组件。',
}
# Windows Python 未必提供 AF_UNIX；Go 桥仍让生产应用使用原有 Unix 客户端。
BRIDGE_SOURCE: str = '''package main

import (
    "net"
    "net/http"
    "net/http/httputil"
    "net/url"
    "os"
)

// main 仅桥接本次隔离 Unix socket 到固定本机协议夹具，不接触 Docker。
func main() {
    target, err := url.Parse(os.Args[2])
    if err != nil { panic(err) }
    listener, err := net.Listen("unix", os.Args[1])
    if err != nil { panic(err) }
    defer listener.Close()
    proxy := httputil.NewSingleHostReverseProxy(target)
    // 丢失已受理回执时关闭原连接，使生产客户端真实观察到传输中断。
    proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, cause error) {
        if hijacker, ok := w.(http.Hijacker); ok {
            connection, _, err := hijacker.Hijack()
            if err == nil { connection.Close(); return }
        }
        http.Error(w, "隔离协议连接中断", http.StatusBadGateway)
    }
    if err := http.Serve(listener, proxy); err != nil { panic(err) }
}
'''


def timestamp() -> str:
    """生成公开任务所用的 UTC 时间，不包含本机私密路径。"""
    return time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())


def request_id() -> str:
    """为每个明确操作生成独立且格式正确的 UUID。"""
    return str(uuid.uuid4())


def write_json(path: Path, value: Any) -> None:
    """在隔离目录原子更新控制或证据文件，避免服务读到半份 JSON。"""
    temporary = path.with_suffix(path.suffix + '.tmp')
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2), encoding='utf-8')
    temporary.replace(path)


class HelperFixture:
    """只模拟固定 helper HTTP 协议，不模拟镜像可运行性或上游产品 API。"""

    def __init__(self, root: Path) -> None:
        """创建有锁状态、独立控制文件和仅回环可访问的协议服务。"""
        # 本次夹具唯一文件根，控制文件不进入生产应用。
        self.root: Path = root
        # 跨 HTTP 工作线程保护任务、计数和请求绑定。
        self.lock: threading.RLock = threading.RLock()
        # 控制项来自本次隔离目录，解析失败时保留最近有效快照。
        self.control: dict[str, Any] = {}
        # 最近处理的重置令牌，显式变化才重置状态。
        self.reset_token: str = ''
        # 固定组件状态的内存快照。
        self.components: dict[str, dict[str, Any]] = {}
        # 任务与增量事件，私有以下划线开头的字段不会进入 DTO。
        self.tasks: dict[str, dict[str, Any]] = {}
        # 请求 UUID 与意图签名及任务标识的持久夹具绑定。
        self.bindings: dict[str, tuple[str, str]] = {}
        # 已消费的一次性丢响应或失败令牌。
        self.consumed: set[str] = set()
        # helper 实际收到的 POST 次数，与创建执行任务数分开。
        self.posts: int = 0
        # 新创建部署任务数，用于证明 UUID 回放没有二次执行。
        self.accepted: int = 0
        # 实际收到的 retry 次数，用于验证 app 先检查作用域。
        self.retries: int = 0
        # 显式启动原容器的次数。
        self.starts: int = 0
        self.reset()
        owner = self

        class Handler(http.server.BaseHTTPRequestHandler):
            """接收固定协议请求并禁止默认输出原始正文或凭据。"""

            def log_message(self, format: str, *args: Any) -> None:
                """抑制标准库访问日志，公开证据仅保留受控中文断言。"""

            def reply(self, status: int, value: Any) -> None:
                """发送有界 JSON 协议回执并明确关闭连接。"""
                body = json.dumps(value, ensure_ascii=False).encode('utf-8')
                try:
                    self.send_response(status)
                    self.send_header('Content-Type', 'application/json; charset=utf-8')
                    self.send_header('Content-Length', str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)
                except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
                    # 服务切换会主动中止旧读取；只忽略已断开连接的回执收尾。
                    self.close_connection = True

            def do_GET(self) -> None:
                """把只读请求交给固定协议分派，不执行部署副作用。"""
                self.dispatch()

            def do_POST(self) -> None:
                """把明确动作请求交给固定协议分派。"""
                self.dispatch()

            def dispatch(self) -> None:
                """读取安全 JSON 并支持已受理后丢失回执的传输场景。"""
                length = int(self.headers.get('Content-Length', '0'))
                if length > 65536:
                    self.reply(413, {'error': '隔离协议请求过大'})
                    return
                try:
                    body = json.loads(self.rfile.read(length)) if length else {}
                except (ValueError, OSError):
                    self.reply(400, {'error': '隔离协议 JSON 无效'})
                    return
                code, value, drop = owner.handle(self.command, self.path, body)
                if drop:
                    # 真实关闭上游连接，桥接器再关闭应用看到的原 Unix 连接。
                    self.close_connection = True
                    self.connection.shutdown(socket.SHUT_RDWR)
                    self.connection.close()
                    return
                self.reply(code, value)

        # 夹具网络监听仅用于短生命周期 Unix 桥，不注入生产 TCP 地址。
        self.server: http.server.ThreadingHTTPServer = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        # 关闭监听时等待本次请求线程收尾，避免超时场景留下后台线程。
        self.server.daemon_threads = False
        # 夹具服务地址由系统分配空闲端口。
        self.base: str = f'http://127.0.0.1:{self.server.server_port}'
        # 可回收的服务线程，close 负责等待退出。
        self.thread: threading.Thread = threading.Thread(target=self.server.serve_forever, name='components-helper-fixture', daemon=True)
        self.thread.start()

    def reset(self) -> None:
        """重置固定组件和隔离任务，只允许控制文件显式触发。"""
        with self.lock:
            self.components = {
                component: {'id': component, 'name': fields[0], 'version': fields[1], 'presence': 'missing', 'runtime': 'unknown',
                            'readiness': 'not_configured', 'message': '隔离协议夹具：尚未部署', 'revision': 1, 'canDeploy': True,
                            'canStart': False, 'setupPort': fields[2], 'serviceUrl': '', 'latestTaskId': ''}
                for component, fields in COMPONENTS.items()
            }
            self.tasks.clear()
            self.bindings.clear()
            self.consumed.clear()
            for task_id, action, component in (('old-qb-task-001', 'initialize', 'qbittorrent'), ('old-stop-task-001', 'stop', 'moviepilot')):
                self.tasks[task_id] = self.new_task_value(task_id, action, component, 'failed')

    def set_control(self, value: dict[str, Any]) -> None:
        """原子修改本次夹具控制项，参数含义见 serve 输出的控制说明。"""
        write_json(self.root / 'control.json', value)

    def load_control(self) -> None:
        """读取人工控制快照，允许浏览器验收切换离线、失败和运行状态。"""
        try:
            candidate = json.loads((self.root / 'control.json').read_text(encoding='utf-8-sig'))
            if isinstance(candidate, dict):
                self.control = candidate
        except (OSError, ValueError):
            pass
        token = str(self.control.get('resetToken', ''))
        if token and token != self.reset_token:
            self.reset()
            self.reset_token = token

    def new_task_value(self, task_id: str, action: str, component: str, state: str = 'queued') -> dict[str, Any]:
        """建立真实 DTO 形状，私有时间与失败控制始终过滤。"""
        created = timestamp()
        return {'id': task_id, 'action': action, 'state': state, 'revision': 1, 'components': [component],
                'steps': [{'component': component, 'phase': 'queued', 'state': state, 'attempt': 1, 'message': '隔离协议任务等待执行', 'layers': []}],
                'createdAt': created, 'updatedAt': created, 'message': '隔离协议任务等待执行', '_started': time.monotonic(), '_events': [], '_fail': False}

    def event(self, task: dict[str, Any], message: str) -> None:
        """追加递增游标的中文安全事件，不写入凭据或上游原始响应。"""
        events = task['_events']
        events.append({'sequence': len(events) + 1, 'component': task['components'][0], 'level': 'error' if task['state'] == 'failed' else 'info',
                       'message': message, 'createdAt': timestamp()})

    def advance(self) -> None:
        """按可控时间推进协议任务；分层字节只代表夹具数据。"""
        delay = max(.1, min(float(self.control.get('stepSeconds', .35)), 30))
        for task in self.tasks.values():
            if task['state'] not in ('queued', 'running'):
                continue
            age = time.monotonic() - task['_started']
            if task['state'] == 'queued' and age >= .05:
                task['state'] = 'running'
                task['revision'] += 1
                task['steps'][0].update({'state': 'running', 'phase': 'pulling', 'message': '隔离夹具正在报告镜像层进度',
                                         'layers': [{'id': 'fixture-layer', 'status': '下载中', 'current': 1024, 'total': 4096}]})
                self.event(task, '隔离夹具开始执行固定组件任务')
            if age < delay or self.control.get('hold', False):
                continue
            failed = bool(task['_fail'] or self.control.get('fail', False))
            task['state'] = 'failed' if failed else 'completed'
            task['revision'] += 1
            task['updatedAt'] = timestamp()
            task['message'] = '隔离夹具：部署失败，可显式重试' if failed else '隔离夹具：容器就绪，仍需首次配置和 API 连接'
            task['steps'][0].update({'state': task['state'], 'phase': 'pulling' if failed else 'completed', 'message': task['message'], 'layers': []})
            self.event(task, task['message'])
            component = self.components.get(task['components'][0])
            if component:
                component['revision'] += 1
                if not failed:
                    component.update({'presence': 'present', 'runtime': 'running', 'readiness': 'configuration_required', 'message': task['message'],
                                      'canDeploy': False, 'canStart': False, 'serviceUrl': f'http://nas-engine-fixture000001-{component["id"]}:{COMPONENTS[component["id"]][3]}'})

    def effective_component(self, component: str) -> dict[str, Any]:
        """应用白名单状态覆盖，保持组件身份与端口由固定目录派生。"""
        value = copy.deepcopy(self.components[component])
        overrides = self.control.get('components', {}).get(component, {})
        for field in ('presence', 'runtime', 'readiness', 'message', 'revision', 'canDeploy', 'canStart', 'serviceUrl', 'latestTaskId'):
            if field in overrides:
                value[field] = overrides[field]
        return value

    def task_response(self, task: dict[str, Any], after: int = 0) -> dict[str, Any]:
        """返回严格公开 DTO，事件游标由当前任务内递增序号决定。"""
        value = {key: copy.deepcopy(item) for key, item in task.items() if not key.startswith('_')}
        return {'task': value, 'events': [copy.deepcopy(event) for event in task['_events'] if event['sequence'] > after],
                'nextSequence': len(task['_events']), 'reloadRequired': False}

    def stats(self) -> tuple[int, int, int, int]:
        """读取实际接收和受理计数，避免把返回成功当作唯一副作用证据。"""
        with self.lock:
            return self.posts, self.accepted, self.retries, self.starts

    def handle(self, method: str, target: str, body: dict[str, Any]) -> tuple[int, dict[str, Any], bool]:
        """分派四组件协议，保持 UUID 签名、单队列和乐观版本语义。"""
        with self.lock:
            self.load_control()
            self.advance()
            parsed = urllib.parse.urlsplit(target)
            path = parsed.path
            if method == 'POST':
                self.posts += 1
            helper = self.control.get('helper', 'ready')
            if helper == 'unavailable':
                return 503, {'error': '隔离部署助手暂不可用'}, False
            if helper == 'unsupported':
                return 404, {'error': '隔离旧助手没有组件接口'}, False
            if method == 'GET' and path == '/api/collection/components':
                delay = min(20, max(0, float(self.control.get('delayStatusSeconds', 0))))
                if delay:
                    time.sleep(delay)
                active = next((task['id'] for task in self.tasks.values() if task['state'] in ('queued', 'running')), '')
                values = [self.effective_component(component) for component in COMPONENTS]
                if helper == 'not_enabled':
                    for component in values:
                        component.update({'presence': 'unknown', 'canDeploy': False, 'canStart': False, 'serviceUrl': ''})
                return 200, {'helper': helper, 'message': '隔离 helper 协议夹具；没有执行 Docker', 'templateVersion': TEMPLATE_VERSION,
                             'noticeVersion': NOTICE_VERSION, 'noticeText': '部署会创建固定组件和私密配置卷，并允许读写 /downloads 与 /data。跨目录整理请选择复制。',
                             'activeTaskId': active, 'components': values}, False
            parts = path.strip('/').split('/')
            if method == 'GET' and len(parts) == 5 and parts[:4] == ['api', 'collection', 'components', 'tasks']:
                task = self.tasks.get(parts[4])
                if not task or parts[4] in self.control.get('goneTasks', []):
                    return 404, {'error': '隔离任务不存在'}, False
                after = int(urllib.parse.parse_qs(parsed.query).get('afterSequence', ['0'])[0])
                response = self.task_response(task, after)
                if self.control.get('malformedTask', False):
                    response['unexpectedField'] = True
                return 200, response, False
            deploy = method == 'POST' and path == '/api/collection/components/deploy'
            start = method == 'POST' and len(parts) == 5 and parts[:3] == ['api', 'collection', 'components'] and parts[4] == 'start'
            retry = method == 'POST' and len(parts) == 6 and parts[:4] == ['api', 'collection', 'components', 'tasks'] and parts[5] == 'retry'
            if not (deploy or start or retry):
                return 404, {'error': '隔离协议不支持此路径'}, False
            if retry:
                self.retries += 1
            signature = method + path + json.dumps({key: value for key, value in body.items() if key != 'requestId'}, sort_keys=True)
            binding = self.bindings.get(body.get('requestId', ''))
            if binding:
                if binding[0] != signature:
                    return 409, {'error': '请求编号已绑定其他意图'}, False
                return 202, self.task_response(self.tasks[binding[1]]), False
            previous = self.tasks.get(parts[4]) if retry else None
            component = str(body.get('component', '')) if deploy else (previous['components'][0] if previous else parts[3])
            if component not in COMPONENTS:
                return 400, {'error': '不支持的固定组件'}, False
            if deploy and (not body.get('acceptNotice') or body.get('templateVersion') != TEMPLATE_VERSION or body.get('noticeVersion') != NOTICE_VERSION):
                return 409, {'error': '请重新确认模板和目录授权'}, False
            if start and body.get('revision') != self.effective_component(component)['revision']:
                return 409, {'error': '组件版本已变化，请重新检测'}, False
            if retry and (not previous or body.get('revision') != previous['revision'] or previous['state'] not in ('failed', 'interrupted')):
                return 409, {'error': '任务版本或可重试状态已变化'}, False
            active = next((task for task in self.tasks.values() if task['state'] in ('queued', 'running')), None)
            if active:
                if deploy and active['action'] == 'deploy' and active['components'] == [component]:
                    self.bindings[body['requestId']] = (signature, active['id'])
                    return 202, self.task_response(active), False
                return 429, {'error': '另一个固定组件任务正在执行'}, False
            if start and not self.effective_component(component)['canStart']:
                return 409, {'error': '原受管容器当前不能启动'}, False
            if deploy and not self.effective_component(component)['canDeploy']:
                return 409, {'error': '固定组件已经存在'}, False
            if retry and previous:
                task = previous
                task.update({'state': 'queued', 'revision': task['revision'] + 1, '_started': time.monotonic(), '_fail': False})
                task['steps'][0].update({'state': 'queued', 'attempt': task['steps'][0]['attempt'] + 1, 'layers': []})
                self.event(task, '隔离夹具已受理失败步骤重试')
            else:
                action = 'deploy' if deploy else 'start'
                task_id = 'fixture-' + uuid.uuid4().hex
                task = self.new_task_value(task_id, action, component)
                self.tasks[task_id] = task
                self.event(task, '隔离夹具已受理固定组件任务')
                if deploy:
                    self.accepted += 1
                else:
                    self.starts += 1
                fail_token = str(self.control.get('failNextToken', ''))
                if fail_token and 'fail:' + fail_token not in self.consumed:
                    self.consumed.add('fail:' + fail_token)
                    task['_fail'] = True
            self.components[component]['latestTaskId'] = task['id']
            self.bindings[body['requestId']] = (signature, task['id'])
            drop_token = str(self.control.get('dropDeployToken', ''))
            drop = bool(deploy and drop_token and 'drop:' + drop_token not in self.consumed)
            if drop:
                self.consumed.add('drop:' + drop_token)
            return 202, self.task_response(task), drop

    def close(self) -> None:
        """关闭仅本次拥有的回环监听并回收服务线程。"""
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)


class ComponentHarness:
    """启动真实应用与 Unix 协议桥，保留隔离证据供后续浏览器验收。"""

    def __init__(self, binary: Path, root: Path) -> None:
        """准备独立数据、媒体、连接目录和有限生命周期进程。"""
        # 已由调用方构建的本机应用。
        self.binary: Path = binary
        # 必须全新且包含 verify 段的隔离目录。
        self.root: Path = root
        # 固定 helper socket 路径，只由本次桥接进程创建。
        self.socket: Path = root / 'h.sock'
        # 真实应用的独立进程。
        self.process: subprocess.Popen[bytes] | None = None
        # 私有 Unix 桥接独立进程。
        self.bridge: subprocess.Popen[bytes] | None = None
        # 应用和桥接器日志文件句柄，close 统一回收。
        self.logs: list[Any] = []
        # 实际执行过的断言结果数量由通用 Client 维护。
        self.client: Client
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            port = listener.getsockname()[1]
        # 独立同源根，cookie 不与开发服务共享。
        self.base: str = f'http://127.0.0.1:{port}'
        self.client = Client(self.base)
        # 仅供浏览器验收登录的本次随机账号，绝不打印密码。
        self.credentials: dict[str, str] = {'username': 'components-verify', 'password': generate_password()}
        # 隔离 NAS 配置从清理过的父环境构造，避免连接用户服务。
        self.env: dict[str, str] = {key: value for key, value in os.environ.items() if not key.startswith('NAS_')}
        for directory in ('data', 'media', 'connections'):
            (root / directory).mkdir(parents=True)
        write_json(root / 'credentials.json', self.credentials)
        if os.name != 'nt':
            (root / 'credentials.json').chmod(0o600)
        self.env.update({
            # 动态回环监听，防止对外发布隔离账户。
            'NAS_ADDR': f'127.0.0.1:{port}',
            # 本次 SQLite 和衍生缓存的唯一位置。
            'NAS_DATA_DIR': str(root / 'data'),
            # 唯一授权空媒体根，完全不接触用户媒体。
            'NAS_MEDIA_ROOTS': str(root / 'media'),
            # 复用调用方已构建的本仓库前端。
            'NAS_WEB_DIR': str(Path(__file__).resolve().parents[1] / 'web' / 'dist'),
            # 延后周期扫描，验证步骤显式控制 API。
            'NAS_SCAN_INTERVAL_MINUTES': '60',
            # 小样本不保留额外磁盘空间。
            'NAS_MIN_FREE_MB': '0',
            # 关闭用户下载引擎，组件部署协议不接管下载任务。
            'NAS_DOWNLOAD_MODE': 'disabled',
            # 关闭真实站点搜索，验证不会访问外部来源。
            'NAS_TORRENT_SEARCH_MODE': 'disabled',
            # 仅使用本次拥有的 Unix socket，保持生产传输路径。
            'NAS_ENGINE_HELPER_SOCKET': str(self.socket),
            # 私密连接根为空，不让 helper 就绪冒充已有 API 连接。
            'NAS_ENGINE_CONNECTION_DIR': str(root / 'connections'),
        })
        # 受控 helper 只模拟协议和任务状态，未连接 Docker。
        self.fixture: HelperFixture = HelperFixture(root)
        self.fixture.set_control({})

    def start_bridge(self) -> None:
        """编译隔离 Go 桥并确认 Unix socket 已实际监听。"""
        repository = Path(__file__).resolve().parents[1]
        go = repository / '.tools' / 'go' / 'bin' / ('go.exe' if os.name == 'nt' else 'go')
        command = str(go) if go.is_file() else shutil.which('go')
        if not command:
            raise RuntimeError('找不到 Go，无法建立生产 Unix 协议夹具')
        source = self.root / 'helper-fixture.go'
        executable = self.root / ('helper-fixture.exe' if os.name == 'nt' else 'helper-fixture')
        source.write_text(BRIDGE_SOURCE, encoding='utf-8')
        subprocess.run([command, 'build', '-o', str(executable), str(source)], cwd=repository, check=True)
        # 可复用源码已保留在当前脚本，构建完成删除本次生成的临时 Go 源。
        source.unlink()
        log = (self.root / 'bridge.log').open('ab')
        self.logs.append(log)
        self.bridge = subprocess.Popen([str(executable), str(self.socket), self.fixture.base], stdout=log, stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            if self.bridge.poll() is not None:
                raise RuntimeError('Unix 协议桥提前退出，请检查隔离 bridge.log')
            if self.socket.exists():
                return
            time.sleep(.05)
        raise RuntimeError('Unix 协议桥监听超时')

    def start(self) -> None:
        """启动真实应用并有限等待健康接口，重复调用保留原数据库。"""
        log = (self.root / 'application.log').open('ab')
        self.logs.append(log)
        self.process = subprocess.Popen([str(self.binary)], env=self.env, stdout=log, stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            if self.process.poll() is not None:
                raise RuntimeError('应用提前退出，请检查隔离 application.log')
            try:
                if self.client.request('/healthz')[0] == 200:
                    return
            except OSError:
                pass
            time.sleep(.1)
        raise RuntimeError('真实应用启动超时')

    def stop(self) -> None:
        """仅终止本次启动的应用，并保留数据库用于重启验证。"""
        self.stop_process(self.process)
        self.process = None

    def stop_process(self, process: subprocess.Popen[bytes] | None) -> None:
        """回收拥有的单个进程，超时后只强制结束该进程。"""
        if process and process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=20)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)

    def deploy_body(self, component: str) -> dict[str, Any]:
        """创建默认明确授权请求，具体拒绝用例单独覆盖字段。"""
        return {'requestId': request_id(), 'component': component, 'templateVersion': TEMPLATE_VERSION, 'noticeVersion': NOTICE_VERSION, 'acceptNotice': True}

    def wait_task(self, task_id: str, state: str = 'completed') -> dict[str, Any]:
        """有限轮询公开任务，状态不符时输出安全 DTO。"""
        deadline = time.monotonic() + 10
        last: dict[str, Any] = {}
        while time.monotonic() < deadline:
            last = self.client.json('/api/collection/components/tasks/' + task_id)
            if last['task']['state'] == state:
                return last
            time.sleep(.1)
        raise AssertionError('任务未收敛到 ' + state + '：' + json.dumps(last, ensure_ascii=False))

    def component(self, component: str) -> dict[str, Any]:
        """从真实 app 转发状态中读取指定固定组件。"""
        return next(value for value in self.client.json('/api/collection/components')['components'] if value['id'] == component)

    def run(self) -> None:
        """执行真实应用边界和受控 helper 恢复协议的可复用集成验收。"""
        self.start_bridge()
        self.start()
        client = self.client
        anonymous = Client(self.base)
        route = '/api/collection/components'
        deploy = route + '/deploy'
        old_task = route + '/tasks/old-qb-task-001'
        body = self.deploy_body('moviepilot')
        self.client.json('/api/setup', 'POST', self.credentials)
        before = self.fixture.stats()
        for path, method, payload in ((route, 'GET', None), (deploy, 'POST', body), (route + '/moviepilot/start', 'POST', {'requestId': request_id(), 'revision': 1}),
                                      (old_task, 'GET', None), (old_task + '/retry', 'POST', {'requestId': request_id(), 'revision': 1})):
            client.check(anonymous.request(path, method, payload)[0] == 401, '组件接口要求登录：' + method + ' ' + path)
        client.check(self.fixture.stats() == before, '未认证请求没有到达私有 helper')
        client.check(client.request(deploy, 'POST', body, {'Origin': 'https://foreign.invalid'})[0] == 403, '跨域部署被同源边界拒绝')
        client.check(client.request(deploy, 'POST', body, {'Sec-Fetch-Site': 'cross-site'})[0] == 403, '跨站部署被请求来源边界拒绝')
        for payload in ({**body, 'image': 'arbitrary/image'}, {**body, 'command': 'arbitrary'}, {**body, 'component': 'qbittorrent'}, {**body, 'requestId': 'bad/id'}):
            client.check(client.request(deploy, 'POST', payload)[0] == 400, '严格拒绝未知参数、旧引擎或不安全请求编号')
        client.check(client.request(route + '/moviepilot/start', 'POST', {'requestId': request_id(), 'revision': 1, 'command': 'arbitrary'})[0] == 400, '启动请求拒绝未知字段')
        client.check(client.request(old_task + '/retry', 'POST', {'requestId': request_id(), 'revision': 1, 'extra': True})[0] == 400, '重试请求拒绝未知字段')
        for payload in ({**body, 'acceptNotice': False}, {**body, 'templateVersion': 'old'}, {**body, 'noticeVersion': 'old'}):
            client.check(client.request(deploy, 'POST', payload)[0] == 409, '未确认或过期的模板与目录授权不能部署')
        client.check(self.fixture.stats() == before, '同源、严格字段和授权拒绝没有触发 helper 动作')
        for _ in range(3):
            status = client.json(route)
        client.check([value['id'] for value in status['components']] == list(COMPONENTS) and all(value['canDeploy'] for value in status['components']), '只读状态包含四个固定可部署组件')
        client.check(self.fixture.stats() == before and status['activeTaskId'] == '', '重复打开和检测不会创建部署任务')
        self.fixture.set_control({'hold': True})
        result = client.json(deploy, 'POST', body, status=202)
        task_id = result['task']['id']
        client.check(result['task']['action'] == 'deploy' and result['task']['components'] == ['moviepilot'], '明确确认创建单个固定组件部署任务')
        replay = client.json(deploy, 'POST', body, status=202)
        client.check(replay['task']['id'] == task_id and self.fixture.accepted == 1, '同 UUID 回放恢复原任务且只受理一次')
        client.check(client.request(deploy, 'POST', {**body, 'component': 'radarr'})[0] == 409, '同 UUID 改变组件意图返回冲突')
        alias = client.json(deploy, 'POST', self.deploy_body('moviepilot'), status=202)
        client.check(alias['task']['id'] == task_id and self.fixture.accepted == 1, '同目标活动请求复用原任务')
        client.check(client.request(deploy, 'POST', self.deploy_body('radarr'))[0] == 429, '其他目标忙碌时不创建第二个执行任务')
        running = self.wait_task(task_id, 'running')
        client.check(running['task']['steps'][0]['layers'][0]['current'] == 1024, 'app 保留 helper 报告的分层字节进度')
        cursor = running['nextSequence']
        current = client.json(route + '/tasks/' + task_id + '?afterSequence=' + str(cursor))
        client.check(current['events'] == [] and current['nextSequence'] == cursor, '增量事件游标不会重复返回旧事件')
        for suffix in ('?afterSequence=-1', '?afterSequence=0&extra=true', '?afterSequence=0&afterSequence=1'):
            client.check(client.request(route + '/tasks/' + task_id + suffix)[0] == 400, '拒绝无效、额外或重复的事件游标')
        self.fixture.set_control({})
        self.wait_task(task_id)
        value = self.component('moviepilot')
        sources = client.json('/api/collection/sources')['sources']
        client.check(value['readiness'] == 'configuration_required' and value['serviceUrl'].endswith(':3000') and sources == [], '容器响应仅表示待首次配置，不创建 API 已连接来源')
        before_retry = self.fixture.retries
        for blocked in ('old-qb-task-001', 'old-stop-task-001'):
            path = route + '/tasks/' + blocked
            client.check(client.request(path)[0] == 404, '新任务路由拒绝旧引擎或完整停止任务')
            client.check(client.request(path + '/retry', 'POST', {'requestId': request_id(), 'revision': 1})[0] == 404, '新重试路由先检查任务作用域')
        client.check(self.fixture.retries == before_retry, '被拒绝作用域没有转发 helper 重试副作用')
        self.fixture.set_control({'failNextToken': 'autobangumi-failure'})
        failed_id = client.json(deploy, 'POST', self.deploy_body('autobangumi'), status=202)['task']['id']
        failed = self.wait_task(failed_id, 'failed')['task']
        retry_path = route + '/tasks/' + failed_id + '/retry'
        client.check(client.request(retry_path, 'POST', {'requestId': request_id(), 'revision': failed['revision'] + 99})[0] == 409, '过期任务版本不能覆盖失败重试意图')
        client.check(client.request(retry_path + '?afterSequence=0', 'POST', {'requestId': request_id(), 'revision': failed['revision']})[0] == 400, '重试动作拒绝读事件查询参数')
        self.fixture.set_control({})
        retried = client.json(retry_path, 'POST', {'requestId': request_id(), 'revision': failed['revision']}, status=202)
        client.check(retried['task']['id'] == failed_id and retried['task']['steps'][0]['attempt'] == 2, '当前任务版本显式重试保留原任务身份')
        self.wait_task(failed_id)
        sonarr_id = client.json(deploy, 'POST', self.deploy_body('sonarr'), status=202)['task']['id']
        self.wait_task(sonarr_id)
        sonarr = self.component('sonarr')
        self.fixture.set_control({'components': {'sonarr': {'runtime': 'stopped', 'canStart': True, 'serviceUrl': ''}}})
        start_path = route + '/sonarr/start'
        start_body = {'requestId': request_id(), 'revision': sonarr['revision']}
        client.check(client.request(start_path, 'POST', {**start_body, 'revision': sonarr['revision'] + 99})[0] == 409, '过期组件版本不能启动原容器')
        started = client.json(start_path, 'POST', start_body, status=202)
        client.check(started['task']['action'] == 'start' and self.fixture.starts == 1, '当前组件版本只创建原容器启动任务')
        client.check(client.json(start_path, 'POST', start_body, status=202)['task']['id'] == started['task']['id'] and self.fixture.starts == 1, '启动动作相同 UUID 重放不会重复执行')
        self.fixture.set_control({})
        self.wait_task(started['task']['id'])
        lost_body = self.deploy_body('radarr')
        accepted_before = self.fixture.accepted
        self.fixture.set_control({'dropDeployToken': 'radarr-lost-response', 'hold': True})
        client.check(client.request(deploy, 'POST', lost_body)[0] == 503, 'helper 已受理后丢失回执返回传输不可用')
        lost_task_id = self.fixture.bindings[lost_body['requestId']][1]
        self.stop()
        self.start()
        client.check(client.json(route)['activeTaskId'] == lost_task_id, '应用重启仍可从 helper 恢复活动任务')
        recovered = client.json(deploy, 'POST', lost_body, status=202)
        client.check(recovered['task']['id'] == lost_task_id and self.fixture.accepted == accepted_before + 1, '丢响应后原 UUID 跨应用重启找回同一任务')
        self.fixture.set_control({})
        self.wait_task(lost_task_id)
        client.check(all(self.component(component)['readiness'] == 'configuration_required' for component in COMPONENTS), '四类固定组件完成后均保持首次配置门槛')
        self.fixture.set_control({'malformedTask': True})
        client.check(client.request(route + '/tasks/' + task_id)[0] == 502, 'helper 出现未知响应字段时严格拒绝协议漂移')
        self.fixture.set_control({'goneTasks': [task_id]})
        client.check(client.request(route + '/tasks/' + task_id)[0] == 404, '已失效任务回执保留明确的 404 恢复边界')
        self.fixture.set_control({'helper': 'unavailable'})
        unavailable = client.json(route)
        client.check(unavailable['helper'] == 'unavailable' and all(value['presence'] == 'unknown' and not value['canDeploy'] for value in unavailable['components']), '助手故障显示不可用，不能冒充组件缺失并允许部署')
        self.fixture.set_control({'helper': 'unsupported'})
        client.check(client.json(route)['helper'] == 'unsupported', '旧助手无组件接口时保留可操作的更新说明')
        self.fixture.set_control({'helper': 'not_enabled'})
        disabled = client.json(route)
        client.check(disabled['helper'] == 'not_enabled' and all(not component['canDeploy'] and not component['canStart'] for component in disabled['components']), '部署能力未启用时固定组件不可执行')
        self.fixture.set_control({'delayStatusSeconds': 11})
        delayed_start = time.monotonic()
        delayed = client.json(route)
        client.check(delayed['helper'] == 'ready' and time.monotonic() - delayed_start >= 10.5, '实际十一秒状态响应仍通过新 helper 控制面超时预算')
        self.fixture.set_control({})
        self.stop_process(self.bridge)
        if self.socket.exists():
            self.socket.unlink()
        offline = client.json(route)
        client.check(offline['helper'] == 'unavailable' and all(not value['canDeploy'] for value in offline['components']), '真实 Unix 监听离线时明确返回助手不可用')
        self.start_bridge()
        client.check(client.json('/api/collection/sources')['sources'] == [], '部署、启动、重试及应用重启均未绕过原 API 来源检测')
        self.stop()
        log = (self.root / 'application.log').read_text(encoding='utf-8')
        client.check(self.credentials['password'] not in log, '应用关键日志没有泄漏验证登录密码')
        self.start()
        print(f'资源组件集成验证通过：{client.passed} 项；使用真实应用与受控 helper 协议，未执行 Docker。', flush=True)

    def serve(self) -> None:
        """重置成全新可部署界面，供后续浏览器复用并等待明确停止文件。"""
        self.fixture.set_control({'resetToken': request_id(), 'stepSeconds': 2})
        self.client.json('/api/collection/components')
        write_json(self.root / 'fixture-info.json', {'base': self.base, 'root': str(self.root), 'credentialFile': str(self.root / 'credentials.json'),
                   'controlFile': str(self.root / 'control.json'), 'stopFile': str(self.root / 'stop'), 'protocol': 'controlled helper fixture over Unix socket', 'dockerExecuted': False})
        write_json(self.root / 'control-help.json', CONTROL_HELP)
        print('浏览器验收夹具已就绪：' + str(self.root / 'fixture-info.json'), flush=True)
        while not (self.root / 'stop').exists():
            if self.process and self.process.poll() is not None:
                raise RuntimeError('浏览器验收期间应用意外退出')
            time.sleep(.5)

    def close(self) -> None:
        """回收本次进程、监听和线程，仅删除本次拥有的 socket。"""
        self.stop()
        self.stop_process(self.bridge)
        self.fixture.close()
        for log in self.logs:
            log.close()
        if self.socket.exists():
            self.socket.unlink()


def main() -> None:
    """解析中文参数并在成功或失败时都留下不含凭据的验证结果。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True, help='已经构建的本机栖库可执行程序')
    parser.add_argument('--root', type=Path, default=verification_directory() / ('components-' + time.strftime('%Y%m%d-%H%M%S')), help='必须不存在且含 verify 路径段的隔离目录')
    parser.add_argument('--serve', action='store_true', help='验证通过后重置夹具并保持浏览器验收服务，创建根目录 stop 文件后退出')
    args = parser.parse_args()
    root = args.root.resolve()
    if root.exists() or 'verify' not in root.parts or not args.binary.is_file():
        parser.error('二进制必须存在；隔离根必须不存在且包含 verify 路径段')
    if len(str(root / 'h.sock').encode('utf-8')) > 100:
        parser.error('隔离根路径过长，Unix socket 路径必须不超过 100 字节')
    root.mkdir(parents=True)
    harness: ComponentHarness | None = None
    completed = False
    try:
        harness = ComponentHarness(args.binary.resolve(), root)
        harness.run()
        completed = True
        write_json(root / 'result.json', {'passed': True, 'assertions': harness.client.passed, 'realApplication': True,
                                        'helper': 'controlled protocol fixture over private Unix socket', 'dockerExecuted': False})
        if args.serve:
            harness.serve()
    finally:
        if harness:
            harness.close()
        write_json(root / 'result.json', {'passed': completed, 'assertions': harness.client.passed if harness else 0, 'realApplication': True,
                                        'helper': 'controlled protocol fixture over private Unix socket', 'dockerExecuted': False})


if __name__ == '__main__':
    from project_env import run_cli

    raise SystemExit(run_cli(main))
