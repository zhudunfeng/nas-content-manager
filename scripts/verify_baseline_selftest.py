"""以真实短命子进程验证编排边界；保留为可复跑集成自检，不生成单测类。"""

from __future__ import annotations

import sys

sys.dont_write_bytecode = True

import argparse
import copy
import json
import os
import subprocess
import threading
import time
from pathlib import Path
from typing import Any, Callable
from unittest.mock import patch

from project_env import ROOT
from verify_baseline import Blocked, OwnedProcess, case_record, clean_environment, digest, display_path, domain_case, new_run_root, now, require_command, reserve_port, run_command, sanitize, sanitize_fixture_logs, sanitize_json, save_manifest, validate_target, write_json


def check(assertion: bool, description: str, results: list[dict[str, Any]]) -> None:
    """保存每条实际断言，失败时不继续伪造成功计数。"""
    results.append({"description": description, "status": "passed" if assertion else "failed"})
    if not assertion:
        raise AssertionError(description)
    print("✓ " + description, flush=True)


def rejected(action: Callable[[], Any], description: str, results: list[dict[str, Any]]) -> None:
    """确认恶意路径和非法档案被明确拒绝。"""
    try:
        action()
    except (Blocked, ValueError, OSError):
        check(True, description, results)
    else:
        check(False, description, results)


def wait_file(path: Path, timeout: float = 8) -> bool:
    """等待真实子进程产生探针，避免固定睡眠造成竞态。"""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if path.is_file():
            return True
        time.sleep(0.03)
    return False


def wait_heartbeat(path: Path, previous: str, timeout: float = 8) -> bool:
    """在有限预算内等待有效心跳变化，跳过文件改写时的暂时空值。"""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            current = path.read_text(encoding="utf-8")
            if current.isascii() and current.isdigit() and current != previous:
                return True
        except OSError:
            pass  # 自有心跳可能尚未发布或正被改写，只在本次预算内重读。
        time.sleep(0.02)
    return False


def cleanup_links(links: list[Path], case: dict[str, Any]) -> None:
    """只移除自检建立的链接，失败时记入结果并继续保存最终证据。"""
    for link in links:
        try:
            if os.name == "nt":
                link.rmdir()
            else:
                link.unlink()
        except OSError as error:
            reason = sanitize("自检链接清理失败：" + str(error))
            case.update({"status": "failed", "reason": reason})
            print(reason, flush=True)


def verify_account_coverage_gate(root: Path, results: list[dict[str, Any]]) -> None:
    """以受控结果测试账号编排的显式离线参数与完整计数门禁，不计产品通过。"""
    fixture = root / "account-coverage"
    (fixture / "account").mkdir(parents=True)
    (fixture / "evidence").mkdir()
    complete: dict[str, Any] = {
        "passed": True, "assertions": 5, "onlineAssertions": 2, "offlineAssertions": 3,
        "accountMutationAPIs": True, "trueSchema4Migration": True, "offlineCLI": True,
        "offlineVerification": {"requested": True, "passed": True, "assertions": 3, "resultPath": "offline/result.json"},
        "fixtureOnly": True, "scope": "controlled-orchestration-fixture",
        "limitations": ["仅受控编排输入，未执行应用、在线接口、离线CLI或真实终端"],
    }
    write_json(fixture / "account/result.json", complete)
    commands: list[list[str]] = []

    def simulated_command(command: list[str], run_root: Path, label: str, timeout: float) -> tuple[dict[str, Any], list[str], list[str]]:
        """只捕获编排参数并返回受控命令结果，不执行应用或伪称真实离线验证。"""
        commands.append(command)
        return {"exitCode": 0, "timedOut": False, "log": "logs/controlled-account.log"}, [], []

    account = case_record("account", True)
    with patch("verify_baseline.run_command", side_effect=simulated_command):
        domain_case(argparse.Namespace(artifacts=fixture, timeout=5), fixture, fixture / "not-executed", account)
    check(len(commands) == 1 and commands[0].count("--offline") == 1
          and account["assertions"] == 5 and "local-offline-maintenance" in account["evidenceKinds"],
          "受控编排结果：账号强制传离线参数且保留在线离线总计", results)
    evidence = fixture / "evidence/account-assertions.json"
    recorded = json.loads(evidence.read_text(encoding="utf-8"))
    check(recorded["result"].get("fixtureOnly") is True
          and recorded["result"].get("scope") == "controlled-orchestration-fixture"
          and set(account["evidenceKinds"]) == {"local-real-application", "historical-schema-fixtures", "local-offline-maintenance"},
          "受控编排结果：分类与fixture限定独立保存，不作为真实产品执行", results)
    write_json(fixture / "evidence/controlled-account-accepted.json", {
        "scope": "controlled-orchestration-fixture", "applicationExecuted": False,
        "classifiedEvidenceKinds": account["evidenceKinds"], "acceptedInput": recorded["result"],
    })
    # 后续反例复用自有输入路径；先只删除本函数刚产生的成功证据，避免旧证据遮蔽漏调用门禁。
    evidence.unlink()
    incomplete: list[dict[str, Any]] = []
    for field in ("passed", "assertions", "onlineAssertions", "offlineAssertions", "accountMutationAPIs", "trueSchema4Migration", "offlineCLI", "offlineVerification"):
        candidate = copy.deepcopy(complete)
        del candidate[field]
        incomplete.append(candidate)
    for field in ("passed", "accountMutationAPIs", "trueSchema4Migration", "offlineCLI"):
        for replacement in (None, False, 1, "true"):
            candidate = copy.deepcopy(complete)
            candidate[field] = replacement
            incomplete.append(candidate)
    for field in ("assertions", "onlineAssertions", "offlineAssertions"):
        for replacement in (None, False, True, 0, -1, float(complete[field]), str(complete[field])):
            candidate = copy.deepcopy(complete)
            candidate[field] = replacement
            incomplete.append(candidate)
    for replacement in (None, False, [], "true"):
        candidate = copy.deepcopy(complete)
        candidate["offlineVerification"] = replacement
        incomplete.append(candidate)
    for field in ("requested", "passed", "assertions"):
        candidate = copy.deepcopy(complete)
        del candidate["offlineVerification"][field]
        incomplete.append(candidate)
        replacements = (None, False, True, 0, -1, 3.0, "3", 2) if field == "assertions" else (None, False, 1, "true")
        for replacement in replacements:
            candidate = copy.deepcopy(complete)
            candidate["offlineVerification"][field] = replacement
            incomplete.append(candidate)
    for field, replacement in (("assertions", 6), ("onlineAssertions", 3), ("offlineAssertions", 2)):
        candidate = copy.deepcopy(complete)
        candidate[field] = replacement
        incomplete.append(candidate)
    incomplete.append({"passed": True, "assertions": 75})
    refused = 0
    for candidate in incomplete:
        write_json(fixture / "account/result.json", candidate)
        rejected_account = case_record("account", True)
        try:
            # 反例也必须经过真实分派，直接测试helper无法发现domain_case漏调用门禁的回归。
            with patch("verify_baseline.run_command", side_effect=simulated_command):
                domain_case(argparse.Namespace(artifacts=fixture, timeout=5), fixture, fixture / "not-executed", rejected_account)
        except RuntimeError:
            if (rejected_account["assertions"] == 0 and not rejected_account["evidence"]
                    and len(rejected_account["commands"]) == 1 and not evidence.exists()):
                refused += 1
    write_json(fixture / "evidence/coverage-rejections.json", {
        "scope": "controlled-orchestration-fixture", "fixtureOnly": True,
        "attempts": len(incomplete), "rejectedThroughDispatch": refused, "applicationExecuted": False,
    })
    check(refused == len(incomplete) and len(commands) == len(incomplete) + 1
          and all(command.count("--offline") == 1 for command in commands),
          "受控编排结果：真实分派拒绝缺失、非正整数或计数失配且不发布成功证据", results)


def run_self_test(requested_root: str | None) -> int:
    """隔离执行失败/超时/链接/进程归属/脱敏/两次运行等完整边界。"""
    try:
        root = new_run_root(requested_root, "baseline-selftest")
    except (OSError, ValueError, Blocked) as error:
        raise Blocked(str(error)) from error
    (root / "logs").mkdir()
    (root / "evidence").mkdir()
    results: list[dict[str, Any]] = []
    manifest: dict[str, Any] = {"formatVersion": 1, "runId": root.name, "suite": "orchestrator-selftest", "startedAt": now(), "finishedAt": None, "status": "not_run", "cases": [case_record("orchestrator", True)], "limitations": ["仅编排集成自检，不代表产品功能或真实NAS通过"]}
    case = manifest["cases"][0]
    case["startedAt"] = now()
    save_manifest(root, manifest)
    observer: OwnedProcess | None = None
    tree: OwnedProcess | None = None
    links: list[Path] = []
    try:
        verify_account_coverage_gate(root, results)
        rejected(lambda: new_run_root(str(root)), "已有运行根不被覆盖", results)
        rejected(lambda: new_run_root(str(ROOT / ".runtime/outside")), "verify以外路径拒绝", results)
        rejected(lambda: new_run_root(".runtime/verify/../escape"), "上级目录穿越拒绝", results)
        for bad_root in (str(root), str(ROOT / ".runtime/outside")):
            record, tail, _ = run_command([sys.executable, "-B", str(ROOT / "scripts/verify_baseline.py"), "--self-test", "--root", bad_root], root, "reject-selftest-" + str(len(results)), 15)
            case["commands"].append(record)
            check(record["exitCode"] == 2 and "Traceback" not in "\n".join(tail), "自检CLI非法根退出2且不产生traceback", results)
        for name in (".. /escape", "name./child", "name /child", "NUL.txt", "file:stream", "child?.txt"):
            rejected(lambda name=name: new_run_root(str(root / name)), "Windows危险路径分量拒绝：" + name, results)
        outside = new_run_root(str(root.parent / (root.name + "-outside")))
        protected_log = outside / "private.log"
        protected_log.write_text("password=mustRemainUnchanged", encoding="utf-8")
        link = root / "link"
        if os.name == "nt":
            # Windows junction不依赖开发者模式；只指向本次自检目录，结束时仅移除链接本身。
            outcome = subprocess.run(["cmd.exe", "/d", "/c", "mklink", "/J", str(link), str(outside)], capture_output=True, timeout=5, check=False)
            if outcome.returncode != 0:
                raise RuntimeError("无法建立受控junction，不能把链接安全测试标为通过")
        else:
            link.symlink_to(outside, target_is_directory=True)
        links.append(link)
        rejected(lambda: new_run_root(str(link / "escape")), "符号链接或junction祖先拒绝", results)
        original_digest = digest(protected_log)
        sanitize_fixture_logs(root)
        check(digest(protected_log) == original_digest, "二次脱敏不穿过junction修改运行根外日志", results)
        cleanup_case = case_record("link-cleanup-probe", True)
        with patch.object(Path, "rmdir" if os.name == "nt" else "unlink", side_effect=OSError("受控链接清理失败")):
            cleanup_links([link], cleanup_case)
        cleanup_evidence = root / "evidence/link-cleanup-failure.json"
        write_json(cleanup_evidence, cleanup_case)
        check(json.loads(cleanup_evidence.read_text(encoding="utf-8"))["status"] == "failed" and os.path.lexists(link) and digest(protected_log) == original_digest, "链接清理失败仍可保存failed证据且不修改目标", results)
        first, second = new_run_root(str(root / "first")), new_run_root(str(root / "second"))
        check(first != second and first.is_dir() and second.is_dir(), "两次运行根独立且不冲突", results)
        before = (root / "run-manifest.json").read_bytes()
        record, _, _ = run_command([sys.executable, "-B", str(ROOT / "scripts/verify_baseline.py"), "--artifacts", str(root / "missing"), "--root", str(first / "missing-build")], root, "missing-artifact", 30)
        missing = json.loads((first / "missing-build/run-manifest.json").read_text(encoding="utf-8"))
        check(record["exitCode"] == 2 and missing["cases"][0]["status"] == "blocked", "缺产物清单为blocked且进程退出2", results)
        check(before == (root / "run-manifest.json").read_bytes(), "子运行不覆盖外层清单", results)
        case["commands"].append(record)
        listener = reserve_port(0)
        try:
            port = listener.getsockname()[1]
            rejected(lambda: reserve_port(port), "被占端口明确阻塞，不夺取监听器", results)
            check(listener.getsockname()[1] == port, "原监听器仍存活且保持端口归属", results)
        finally:
            listener.close()
        record, _, _ = run_command([sys.executable, "-B", "-c", "raise SystemExit(7)"], root, "failure", 5)
        case["commands"].append(record)
        check(record["exitCode"] == 7 and not record["timedOut"], "真实失败子进程退出码保留", results)
        record, _, _ = run_command([sys.executable, "-B", "-c", "import time; time.sleep(60)"], root, "timeout", 0.3)
        case["commands"].append(record)
        check(record["timedOut"] and record["exitCode"] != 0 and record["durationSeconds"] < 12, "真实超时子进程有界回收", results)
        record, _, _ = run_command([sys.executable, "-B", "-c", "print('独立项继续执行')"], root, "continuation", 5)
        case["commands"].append(record)
        check(record["exitCode"] == 0, "失败及超时之后独立项仍可执行", results)
        real_stop = OwnedProcess.stop

        def failed_cleanup(process: OwnedProcess) -> None:
            """真实回收后注入清理错误，验证命令证据不会因错误丢失。"""
            real_stop(process)
            raise RuntimeError("受控清理失败")

        with patch.object(OwnedProcess, "stop", failed_cleanup):
            record, _, _ = run_command([sys.executable, "-B", "-c", "print('命令已执行')"], root, "cleanup-error", 5)
        case["commands"].append(record)
        failed = False
        try:
            require_command(record)
        except RuntimeError:
            failed = True
        check(failed and record["exitCode"] == 0 and record.get("cleanupError") == "受控清理失败", "清理错误判失败且命令/退出码/日志记录保留", results)
        heartbeat_probe = root / "heartbeat-probe"
        heartbeat_probe.write_text("", encoding="utf-8")
        check(not wait_heartbeat(heartbeat_probe, "before", 0.06), "空心跳不被误判为有效变化", results)

        def publish_heartbeat_probe() -> None:
            """延迟发布确定的有效心跳，覆盖暂时空文件的读取边界。"""
            time.sleep(0.08)
            heartbeat_probe.write_text("1", encoding="utf-8")

        heartbeat_writer = threading.Thread(target=publish_heartbeat_probe)
        heartbeat_writer.start()
        try:
            changed_heartbeat = wait_heartbeat(heartbeat_probe, "", 2)
        finally:
            heartbeat_writer.join(timeout=2)
        check(changed_heartbeat and not heartbeat_writer.is_alive(), "暂时空心跳会等待有效变化而不固定延时误判", results)
        # 通过心跳而非kill(pid,0)验证存活；Windows该函数的特殊信号语义不能用于探测。
        observer_heartbeat = root / "observer-heartbeat"
        descendant_heartbeat = root / "descendant-heartbeat"
        observer_code = "import pathlib,time,sys\np=pathlib.Path(sys.argv[1])\nwhile True:\n p.write_text(str(time.time_ns()))\n time.sleep(.04)"
        observer = OwnedProcess([sys.executable, "-B", "-c", observer_code, str(observer_heartbeat)], clean_environment(), root / "logs/observer.log")
        child_code = "import subprocess,sys,time\nsubprocess.Popen([sys.executable,'-B','-c',sys.argv[1],sys.argv[2]])\nprint('后代已创建',flush=True)\ntime.sleep(60)"
        tree = OwnedProcess([sys.executable, "-B", "-c", child_code, observer_code, str(descendant_heartbeat)], clean_environment(), root / "logs/owned-tree.log")
        check(wait_file(observer_heartbeat) and wait_file(descendant_heartbeat), "真实独立旁观进程及自有孙进程均已运行", results)
        tree.stop()
        stopped = descendant_heartbeat.read_text(encoding="utf-8")
        observer_before = observer_heartbeat.read_text(encoding="utf-8")
        time.sleep(0.2)
        check(descendant_heartbeat.read_text(encoding="utf-8") == stopped, "自有孙进程停止且不再写入", results)
        check(wait_heartbeat(observer_heartbeat, observer_before) and observer.process.poll() is None, "独立旁观进程持续运行，未被错误回收", results)
        # 正常父进程退出也必须收回仍持有输出管道的孙进程。
        normal_heartbeat = root / "normal-descendant"
        normal_code = "import subprocess,sys,time\nsubprocess.Popen([sys.executable,'-B','-c',sys.argv[1],sys.argv[2]])\ntime.sleep(.3)"
        record, _, _ = run_command([sys.executable, "-B", "-c", normal_code, observer_code, str(normal_heartbeat)], root, "normal-descendants", 5)
        case["commands"].append(record)
        stable = normal_heartbeat.read_text(encoding="utf-8")
        time.sleep(0.15)
        check(record["exitCode"] == 0 and normal_heartbeat.read_text(encoding="utf-8") == stable, "正常父进程完成后也收回遗留后代和管道", results)
        sensitive = 'Authorization: Bearer secret123\nCookie: sid=secret456\npassword="secret789" token=tokensecret https://user:pw@private.invalid/path?q=hidden C:\\Users\\private\\notes\n' + str(ROOT) + '\\private\n/home/private/data knownsecret'
        safe = sanitize(sensitive, ("knownsecret",))
        check(not any(secret in safe for secret in ("secret123", "secret456", "secret789", "tokensecret", "private.invalid", "user:pw", "knownsecret", "C:\\Users", "/home/private", str(ROOT))), "凭据、URL、用户绝对路径和已知秘密脱敏", results)
        for private_path in ("/srv/private library/media.mkv", "/root/private file", "/etc/private config", "C:\\Users\\private library\\secret.txt", r"\\server\private share\secret.txt", "//server/private share/secret.txt"):
            check("private" not in sanitize(private_path), "含空格绝对路径完整遮蔽", results)
        nested = {"value": {"token": "hidden", "nested": ["/srv/private media", {"password": "hidden2"}]}}
        restored = json.loads(json.dumps(sanitize_json(nested)))
        check(restored["value"]["token"] == "<redacted>" and "hidden2" not in json.dumps(restored), "递归脱敏后JSON仍有效且嵌套秘密隐藏", results)
        record, _, _ = run_command([sys.executable, "-B", "-c", "print('password=privateSecret123 https://private.invalid/token')"], root, "sanitization", 5)
        case["commands"].append(record)
        logged = (root / record["log"]).read_text(encoding="utf-8")
        check("privateSecret123" not in logged and "private.invalid" not in logged, "真实子进程持久日志已经脱敏", results)
        target_manifest: dict[str, Any] = {"cases": [case_record("target-profile", True)]}
        validate_target(ROOT / "docs/nas-acceptance.template.json", root, target_manifest)
        check(target_manifest["cases"][0]["status"] == "passed" and all(item["status"] in ("blocked", "not_run") for item in target_manifest["cases"][1:]), "模板结构成功不会制造M1/M2/M3通过", results)
        target = json.loads((ROOT / "docs/nas-acceptance.template.json").read_text(encoding="utf-8"))
        target["gates"][0]["status"] = "passed"
        forged = root / "forged-target.json"
        write_json(forged, target)
        rejected(lambda: validate_target(forged, root, {"cases": [case_record("target-profile", True)]}), "无真实执行轨道不能导入目标通过声明", results)
        check(digest(first / "missing-build/run-manifest.json") != "", "证据文件具有可核对摘要", results)
        case.update({"status": "passed", "reason": "全部编排集成边界实际断言完成"})
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError, AssertionError) as error:
        case.update({"status": "failed", "reason": sanitize(str(error))})
        print("自检失败：" + case["reason"], flush=True)
    finally:
        for owned in (tree, observer):
            if owned:
                try:
                    owned.stop()
                except Exception as error:
                    case.update({"status": "failed", "reason": sanitize("自检清理失败：" + str(error))})
        cleanup_links(links, case)
        case.update({"assertions": sum(item["status"] == "passed" for item in results), "finishedAt": now(), "evidence": ["evidence/selftest-assertions.json", "evidence/link-cleanup-failure.json"], "evidenceKinds": ["orchestrator-integration-selftest"]})
        try:
            write_json(root / "evidence/selftest-assertions.json", results)
        except OSError as error:
            case.update({"status": "failed", "reason": sanitize("自检证据保存失败：" + str(error))})
        code = save_manifest(root, manifest, True)
    print("编排集成自检：" + case["status"] + "；" + str(case["assertions"]) + "项；" + display_path(root / "run-manifest.json"), flush=True)
    return code
