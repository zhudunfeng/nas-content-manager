"""使用真实固定依赖和下载器离线核验桥接，替换元数据/图片传输并禁止真实联网。"""
from __future__ import annotations

import importlib.metadata
import io
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
from types import SimpleNamespace
from typing import Any
from uuid import uuid4

from project_env import run_cli, verification_directory


ROOT: Path = Path(__file__).resolve().parents[1]  # 唯一项目根目录。
BASE: Path = ROOT  # main中替换为仓库.runtime下全新验证目录。
jm: Any = None  # 环境初始化后才导入的真实固定JMComic模块。
Image: Any = None  # 环境初始化后才导入的真实Pillow图片实现。
MODE: str = "normal"  # 控制离线元数据和图片响应的固定分支。
DECODED: list[int] = []  # 实际上游解码执行时的分割数。
PROXIES: list[dict[str, str]] = []  # 实际客户端获得的代理配置。
NETWORK_CALLS: list[str] = []  # 任何意外真实传输尝试均记录并失败。


def deny_network(*args: Any, **kwargs: Any) -> Any:
    """强制阻止此验证过程的真实 HTTP 或套接字连接。"""
    NETWORK_CALLS.append("blocked")
    raise AssertionError("offline check forbids real network")


def make_album() -> jm.JmAlbumDetail:
    """创建真实上游实体，包含两章四页与不参与路径的危险标题。"""
    return jm.JmAlbumDetail("100", "0", "../../离线漫画", [("102", "20", "second"), ("101", "10", "first")],
                            4, "", "", "0", "0", 0, [], [], ["fixture"], ["测试"])


def get_album(self: Any, value: str) -> jm.JmAlbumDetail:
    """以固定实体替代外部漫画元数据，保留实际客户端生命周期。"""
    assert value == "100"
    return make_album()


def get_photo(self: Any, value: str, *args: Any, **kwargs: Any) -> jm.JmPhotoDetail:
    """返回实际章节实体，让真实 check_photo 补全页数组与图片编号。"""
    assert value in ("101", "102")
    return jm.JmPhotoDetail(value, "../unsafe", "100", "10" if value == "101" else "20", scramble_id="0",
                            page_arr=["00001.png", "00002.gif" if value == "101" else "00002.png"],
                            data_original_domain="fixture.invalid")


def image_response(self: Any, url: str) -> jm.JmImageResp:
    """只替换图片传输层，真实传输响应仍由上游执行解码和格式转换。"""
    if MODE == "partial" and "/102/00002" in url:
        raise RuntimeError("fixture raw source error must never escape protocol")
    target: io.BytesIO = io.BytesIO()
    Image.new("RGB", (16, 20), (45, 90, 130)).save(target, "GIF" if ".gif" in url else "PNG")
    return jm.JmImageResp(SimpleNamespace(content=target.getvalue(), status_code=200, text="", url=url))


def api_response(self: Any, url: str, *args: Any, **kwargs: Any) -> Any:
    """只替换搜索 API 的传输结果，保留真实搜索分页和数字重定向处理。"""
    if url == "/setting":
        return SimpleNamespace(model_data=jm.AdvancedDict({"jm3_version": jm.JmMagicConstants.APP_VERSION}),
                               resp=SimpleNamespace(cookies={"fixture": "offline"}))
    if "search_query=100" in url:
        data: dict[str, Any] = {"total": 1, "redirect_aid": "100", "content": []}
    elif "search_query=empty" in url:
        data = {"total": "0", "content": []}
    else:
        data = {"total": "81", "content": [{"id": "100", "name": "离线漫画", "tags": ["fixture"]}]}
    return SimpleNamespace(model_data=jm.AdvancedDict(data))


ORIGINAL_CLIENT: Any = None  # main中保存真实选项创建客户端实现。
ORIGINAL_DECODE: Any = None  # main中保存真实上游图片重排与保存实现。


def capture_client(self: Any, *args: Any, **kwargs: Any) -> Any:
    """捕获实际客户端的显式代理后仍使用原上游构建逻辑。"""
    PROXIES.append(dict(self.client.postman.meta_data.proxies.src_dict))
    return ORIGINAL_CLIENT(self, *args, **kwargs)


def fixed_domains(self: Any, client_key: str) -> list[str]:
    """省略公网域名发现，确保此脚本不联系真实来源。"""
    return ["fixture.invalid"]


def count_decode(cls: Any, count: int, picture: Any, name: str) -> None:
    """记录真实像素解码路径并委托原上游图片算法。"""
    DECODED.append(count)
    ORIGINAL_DECODE(count, picture, name)


def action(namespace: dict[str, Any], request: dict[str, Any], name: str) -> dict[str, Any]:
    """在全新且为空的独占目录执行一次真实桥接协议。"""
    directory: Path = BASE / (name + "-" + str(len(list(BASE.iterdir()))))
    directory.mkdir(parents=True, exist_ok=False)
    os.chdir(directory)
    if request["action"] == "download":
        request["outputDir"] = str(directory)
    return namespace["run"](request)


def main() -> None:
    """覆盖真实依赖导入、搜索、重定向、多章解码、部分失败与协议脱敏。"""
    global BASE, MODE, jm, Image, ORIGINAL_CLIENT, ORIGINAL_DECODE
    import curl_cffi.requests
    import jmcomic as jm
    from PIL import Image

    assert importlib.metadata.version("jmcomic") == "2.7.12"
    BASE = verification_directory() / ("jmcomic-real-bridge-" + uuid4().hex)
    BASE.mkdir(mode=0o700, exist_ok=False)
    ORIGINAL_CLIENT = jm.JmOption.new_jm_client
    ORIGINAL_DECODE = jm.JmImageTool.decode_and_save
    source: str = (ROOT / "internal/collectionworker/jmcomic_bridge.py").read_text(encoding="utf-8")
    # 单独子进程不注入替身，真实核验生产使用的隔离启动参数和stdin/stdout协议。
    probe: subprocess.CompletedProcess[bytes] = subprocess.run(
        [sys.executable, "-I", "-B", "-c", source], input=b'{"action":"probe"}',
        cwd=BASE, capture_output=True, timeout=10, check=True)
    assert json.loads(probe.stdout) == {"version": "2.7.12", "protocolVersion": 1} and not probe.stderr
    curl_cffi.requests.Session.request = deny_network
    socket.create_connection = deny_network
    jm.JmModuleConfig.FLAG_API_CLIENT_AUTO_UPDATE_DOMAIN = False
    jm.JmOption.decide_client_domain = fixed_domains
    jm.JmOption.new_jm_client = capture_client
    jm.JmOption.build_jm_client = capture_client
    jm.JmApiClient.get_album_detail = get_album
    jm.JmApiClient.get_photo_detail = get_photo
    jm.JmApiClient.get_jm_image = image_response
    jm.JmApiClient.req_api = api_response
    jm.JmImageTool.decode_and_save = classmethod(count_decode)
    namespace: dict[str, Any] = {"__name__": "jmcomic_bridge_offline_check"}
    exec(compile(source, "jmcomic_bridge.py", "exec"), namespace)
    proxy: str = "http://worker:fixture@127.0.0.1:12345"
    assert namespace["run"]({"action": "probe"}) == {"version": "2.7.12", "protocolVersion": 1}
    result: dict[str, Any] = action(namespace, {"action": "search", "query": "fixture", "page": 1, "proxy": proxy}, "search")
    assert result["total"] == 81 and result["hasNext"] and len(result["items"]) == 1
    redirect: dict[str, Any] = action(namespace, {"action": "search", "query": "100", "page": 1, "proxy": proxy}, "redirect")
    assert redirect["total"] == 1 and not redirect["hasNext"] and redirect["items"][0]["albumId"] == "100"
    empty: dict[str, Any] = action(namespace, {"action": "search", "query": "empty", "page": 1, "proxy": proxy}, "empty")
    assert empty["total"] == 0 and not empty["hasNext"] and not empty["items"]
    preview: dict[str, Any] = action(namespace, {"action": "album", "albumId": "100", "proxy": proxy}, "album")
    assert preview["chapters"] == ["101", "102"] and preview["pageCount"] == 4
    downloaded: dict[str, Any] = action(namespace, {"action": "download", "albumId": "100", "maxBytes": 1048576, "maxFiles": 4, "proxy": proxy}, "download")
    assert downloaded["files"] == 4 and len(DECODED) == 3 and all(count > 0 for count in DECODED)
    download_dir: Path = Path.cwd()
    files: list[Path] = sorted(download_dir.rglob("*.*"))
    assert [file.relative_to(download_dir).as_posix() for file in files] == ["00001/00001.jpg", "00001/00002.gif", "00002/00001.jpg", "00002/00002.jpg"]
    for file in files:
        with Image.open(file) as picture:
            assert picture.size == (16, 20) and picture.format in ("JPEG", "GIF")
            picture.verify()
    MODE = "partial"
    try:
        action(namespace, {"action": "download", "albumId": "100", "maxBytes": 1048576, "maxFiles": 4, "proxy": proxy}, "partial")
        raise AssertionError("partial success was accepted")
    except Exception as error:
        assert not isinstance(error, AssertionError)
    MODE = "normal"
    for label, byte_limit, file_limit in (("file-budget", 1048576, 3), ("byte-budget", 10, 4)):
        try:
            action(namespace, {"action": "download", "albumId": "100", "maxBytes": byte_limit,
                               "maxFiles": file_limit, "proxy": proxy}, label)
            raise AssertionError("download budget was ignored")
        except Exception as error:
            assert not isinstance(error, AssertionError)
    MODE = "partial"
    partial_protocol: Path = BASE / "partial-protocol"
    partial_protocol.mkdir()
    os.chdir(partial_protocol)
    original_stdin: Any = sys.stdin
    original_stdout: Any = sys.stdout
    original_stderr: Any = sys.stderr
    try:
        sys.stdin = io.TextIOWrapper(io.BytesIO(json.dumps({"action": "download", "albumId": "100", "maxBytes": 1048576,
            "maxFiles": 4, "outputDir": str(partial_protocol), "proxy": proxy}).encode()))
        sys.stdout = io.StringIO()
        sys.stderr = io.StringIO()
        assert namespace["main"]() == 1 and sys.stdout.getvalue() == ""
        assert "fixture raw" not in sys.stderr.getvalue()
    finally:
        sys.stdin, sys.stdout, sys.stderr = original_stdin, original_stdout, original_stderr
    assert PROXIES and all(item == {"http": proxy, "https": proxy} for item in PROXIES)
    assert not NETWORK_CALLS
    report: dict[str, Any] = {"dependency": "jmcomic==2.7.12", "realDownloader": True, "isolatedSubprocessProbe": True,
        "realClientConstruction": True, "networkOverridden": ["domain discovery", "album/photo metadata", "search response", "image response"],
        "realNetworkCalls": len(NETWORK_CALLS), "passed": ["probe", "search pagination", "numeric redirect", "empty search", "album chapters", "4-page 2-chapter real decode", "gif preservation", "partial rejection", "file/byte budget rejection", "safe partial-failure protocol error", "explicit proxy binding"]}
    (BASE / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False))
    print("真实桥接离线核验通过，证据：" + str(BASE / "report.json"))


if __name__ == "__main__":
    raise SystemExit(run_cli(main))
