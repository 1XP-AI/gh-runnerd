"""Offline, non-executing regression probes for the seven issue #79 findings.

Python examples and shell commands supplied to the packet scanner remain data:
the harness parses/inspects them but never evaluates or launches them. The only
child processes created below are literal Git commands against temporary local
repositories owned by these tests.
"""

from __future__ import annotations

import ast
import os
import re
import shlex
import selectors
import signal
import subprocess
import tempfile
import time
import types
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
PACKET_PATH = ROOT / "docs" / "evidence" / "g01-recovery-packet.md"
PACKET_TEXT = PACKET_PATH.read_text(encoding="utf-8")


def _target_names(target: ast.expr) -> set[str]:
    if isinstance(target, ast.Name):
        return {target.id}
    if isinstance(target, (ast.Tuple, ast.List)):
        names: set[str] = set()
        for item in target.elts:
            names.update(_target_names(item))
        return names
    return set()


def _safe_assignment_expression(
    node: ast.AST, namespace: dict[str, object], local_names: set[str] | None = None
) -> bool:
    """Allow only literal/pure constants needed to load scanner definitions."""
    if local_names is None:
        local_names = set()
    if isinstance(node, ast.Constant):
        return True
    if isinstance(node, ast.Name):
        return node.id in namespace or node.id in local_names or node.id == "set"
    if isinstance(node, (ast.List, ast.Tuple, ast.Set)):
        return all(_safe_assignment_expression(item, namespace, local_names) for item in node.elts)
    if isinstance(node, ast.Dict):
        return all(
            (key is None or _safe_assignment_expression(key, namespace, local_names))
            and _safe_assignment_expression(value, namespace, local_names)
            for key, value in zip(node.keys, node.values)
        )
    if isinstance(node, ast.Starred):
        return _safe_assignment_expression(node.value, namespace, local_names)
    if isinstance(node, ast.Attribute):
        if isinstance(node.value, ast.Name) and node.value.id == "re":
            return hasattr(re, node.attr)
        return isinstance(node.value, ast.Name) and node.value.id in local_names and node.attr in {
            "rsplit"
        }
    if isinstance(node, ast.Subscript):
        return _safe_assignment_expression(node.value, namespace, local_names) and _safe_assignment_expression(
            node.slice, namespace, local_names
        )
    if isinstance(node, ast.Call):
        if isinstance(node.func, ast.Name) and node.func.id == "set":
            return not node.args and not node.keywords
        if (
            isinstance(node.func, ast.Attribute)
            and isinstance(node.func.value, ast.Name)
            and node.func.value.id == "re"
            and node.func.attr == "compile"
        ):
            return bool(node.args) and all(
                isinstance(argument, ast.Constant) for argument in node.args
            ) and not node.keywords
        if (
            isinstance(node.func, ast.Attribute)
            and isinstance(node.func.value, ast.Name)
            and node.func.value.id in local_names
            and node.func.attr == "rsplit"
        ):
            return all(
                isinstance(argument, ast.Constant) for argument in node.args
            ) and not node.keywords
        return False
    if isinstance(node, (ast.SetComp, ast.ListComp, ast.GeneratorExp)):
        scoped_names = set(local_names)
        for generator in node.generators:
            scoped_names.update(_target_names(generator.target))
            if not _safe_assignment_expression(generator.iter, namespace, scoped_names):
                return False
            if not all(
                _safe_assignment_expression(condition, namespace, scoped_names)
                for condition in generator.ifs
            ):
                return False
        return _safe_assignment_expression(node.elt, namespace, scoped_names)
    if isinstance(node, (ast.UnaryOp, ast.BinOp, ast.BoolOp, ast.Compare, ast.IfExp)):
        return all(
            _safe_assignment_expression(child, namespace, local_names)
            for child in ast.iter_child_nodes(node)
            if not isinstance(child, (ast.operator, ast.unaryop, ast.boolop, ast.cmpop, ast.Load))
        )
    if isinstance(node, ast.Load):
        return True
    return False


def _scanner_module_source(packet: str) -> str:
    definition = packet.index("def inspect_python_heredoc(body, safe_marker):")
    fence_start = packet.rfind("```sh\n", 0, definition)
    if fence_start < 0 or not packet[fence_start:].startswith("```sh\nset -euo pipefail"):
        raise AssertionError("packet scanner shell fence could not be identified")
    fence_end = packet.index("\n```\n", definition)
    code_start = packet.index("\nimport ast\n", fence_start, definition) + 1
    terminator = packet.rfind("\nPY", code_start, fence_end)
    if terminator < 0:
        raise AssertionError("packet scanner Python heredoc terminator is missing")
    source = packet[code_start:terminator]
    if "def forbidden_command(tokens, depth=0):" not in source:
        raise AssertionError("packet scanner functions were not extracted")
    return source


def _scanner_namespace() -> dict[str, object]:
    source = _scanner_module_source(PACKET_TEXT)
    module = ast.parse(source, filename="<packet-scanner-data>")
    namespace: dict[str, object] = {
        "__builtins__": __builtins__,
        "ast": ast,
        "os": os,
        "re": re,
        "shlex": shlex,
        "subprocess": subprocess,
        "tempfile": tempfile,
        "Path": Path,
        "types": types,
        "source": PACKET_TEXT,
    }
    for statement in module.body:
        if isinstance(statement, (ast.Import, ast.ImportFrom)):
            exec(compile(ast.Module(body=[statement], type_ignores=[]), "<packet-scanner-import>", "exec"), namespace)
        elif isinstance(statement, (ast.FunctionDef, ast.AsyncFunctionDef)):
            exec(compile(ast.Module(body=[statement], type_ignores=[]), "<packet-scanner-function>", "exec"), namespace)
        elif isinstance(statement, (ast.Assign, ast.AnnAssign)):
            if isinstance(statement, ast.Assign):
                names = set().union(*(_target_names(target) for target in statement.targets))
                value = statement.value
            else:
                names = _target_names(statement.target)
                value = statement.value
            if "source" in names or value is None:
                continue
            if _safe_assignment_expression(value, namespace):
                exec(compile(ast.Module(body=[statement], type_ignores=[]), "<packet-scanner-constant>", "exec"), namespace)
    namespace["source"] = PACKET_TEXT
    return namespace


def _verification_module(packet: str) -> ast.Module:
    anchor = "The following dynamic command is the live final-verification template."
    start = packet.index(anchor)
    heredoc_start = packet.index("/opt/homebrew/bin/python3 -I - <<'PY'\n", start)
    code_start = heredoc_start + len("/opt/homebrew/bin/python3 -I - <<'PY'\n")
    code_end = packet.index("\nPY\n", code_start)
    return ast.parse(packet[code_start:code_end], filename="<verification-template-data>")


def _top_level_assignment(module: ast.Module, name: str) -> ast.Assign | ast.AnnAssign:
    for statement in module.body:
        if isinstance(statement, ast.Assign) and any(
            isinstance(target, ast.Name) and target.id == name
            for target in statement.targets
        ):
            return statement
        if isinstance(statement, ast.AnnAssign) and isinstance(statement.target, ast.Name) and statement.target.id == name:
            return statement
    raise AssertionError(f"verification template assignment {name!r} is missing")


def _literal_assignment_value(statement: ast.Assign | ast.AnnAssign) -> object:
    value = statement.value
    if value is None:
        raise AssertionError("verification template assignment has no value")
    return ast.literal_eval(value)


def _safe_environment_mapping(module: ast.Module) -> dict[str, str]:
    statement = _top_level_assignment(module, "git_environment")
    expression = statement.value
    assert expression is not None
    allowed_names = {"os", "git_environment_override_names", "git_child_environment_names"}
    allowed_calls = {"items", "startswith"}
    for node in ast.walk(expression):
        if isinstance(node, ast.Name) and isinstance(node.ctx, ast.Load) and node.id not in allowed_names | {"key", "value"}:
            raise AssertionError("Git child environment expression has an unresolved name")
        if isinstance(node, ast.Call):
            if not isinstance(node.func, ast.Attribute) or node.func.attr not in allowed_calls:
                raise AssertionError("Git child environment expression is not a passive mapping filter")
            if node.func.attr == "items" and not (
                isinstance(node.func.value, ast.Attribute)
                and node.func.value.attr == "environ"
                and isinstance(node.func.value.value, ast.Name)
                and node.func.value.value.id == "os"
            ):
                raise AssertionError("Git child environment items source is not the supplied synthetic map")
            if node.func.attr == "startswith" and not isinstance(node.func.value, ast.Name):
                raise AssertionError("Git child environment prefix check is not passive")
    safe_names: dict[str, object] = {}
    try:
        allowlist = _literal_assignment_value(_top_level_assignment(module, "git_child_environment_names"))
    except AssertionError:
        allowlist = ()
    except (ValueError, TypeError, SyntaxError):
        raise AssertionError("Git child environment allowlist is not literal")
    safe_names["git_child_environment_names"] = allowlist
    safe_names["git_environment_override_names"] = set()
    synthetic_environment = {
        "PATH": "/synthetic/bin",
        "LANG": "C",
        "LC_ALL": "C",
        "GH_TOKEN": "synthetic-only",
        "GITHUB_TOKEN": "synthetic-only",
        "GITHUB_APP_PRIVATE_KEY": "synthetic-only",
        "CUSTOM_SECRET": "synthetic-only",
        "HOME": "/synthetic/home",
    }
    namespace = {
        "__builtins__": {},
        "os": types.SimpleNamespace(environ=synthetic_environment),
        **safe_names,
    }
    result = eval(compile(ast.Expression(expression), "<environment-map>", "eval"), namespace)
    if not isinstance(result, dict):
        raise AssertionError("Git child environment expression did not build a mapping")
    for statement in module.body:
        if not (
            isinstance(statement, ast.Expr)
            and isinstance(statement.value, ast.Call)
            and isinstance(statement.value.func, ast.Attribute)
            and isinstance(statement.value.func.value, ast.Name)
            and statement.value.func.value.id == "git_environment"
            and statement.value.func.attr == "update"
            and len(statement.value.args) == 1
        ):
            continue
        update = ast.literal_eval(statement.value.args[0])
        if not isinstance(update, dict):
            raise AssertionError("Git child environment override is not a literal mapping")
        result.update(update)
    return result


def _verification_function(module: ast.Module, name: str) -> ast.FunctionDef:
    for statement in module.body:
        if isinstance(statement, ast.FunctionDef) and statement.name == name:
            return statement
    raise AssertionError(f"verification helper {name!r} is missing")


def _safe_integer_expression(node: ast.AST) -> int:
    if isinstance(node, ast.Constant) and type(node.value) is int:
        return node.value
    if isinstance(node, ast.BinOp) and isinstance(node.op, ast.Mult):
        return _safe_integer_expression(node.left) * _safe_integer_expression(node.right)
    if isinstance(node, ast.BinOp) and isinstance(node.op, ast.Add):
        return _safe_integer_expression(node.left) + _safe_integer_expression(node.right)
    raise AssertionError("Git query budget/deadline is not a literal integer expression")


def _bounded_git_query_namespace(module: ast.Module) -> dict[str, object]:
    """Load only the reviewed bounded local-Git query helpers from the template."""
    function_names = {
        "close_git_query_streams",
        "git_query_group_exists",
        "wait_for_git_query_group_exit",
        "terminate_git_query_group",
        "capture_git_query_output",
        "run_bounded_git_query",
        "git_query",
        "run_bounded_git_packet_blob_query",
    }
    constant_names = {
        "git_query_deadline_seconds",
        "git_query_termination_grace_seconds",
        "git_query_output_max_bytes",
        "git_query_packet_blob_output_max_bytes",
        "git_query_stream_chunk_bytes",
    }
    namespace: dict[str, object] = {
        "__builtins__": __builtins__,
        "os": os,
        "selectors": selectors,
        "signal": signal,
        "subprocess": subprocess,
        "tempfile": tempfile,
        "time": time,
        "Path": Path,
    }
    for statement in module.body:
        if isinstance(statement, ast.Assign):
            names = {
                target.id
                for target in statement.targets
                if isinstance(target, ast.Name) and target.id in constant_names
            }
            if names:
                value = _safe_integer_expression(statement.value)
                for name in names:
                    namespace[name] = value
        elif isinstance(statement, ast.FunctionDef) and statement.name in function_names:
            exec(
                compile(ast.Module(body=[statement], type_ignores=[]), "<bounded-git-query>", "exec"),
                namespace,
            )
    return namespace


def _run_local_git(arguments: list[str], cwd: Path, env: dict[str, str]) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        ["git", *arguments], cwd=cwd, env=env, stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
    )


def _run_git_checked(arguments: list[str], cwd: Path, env: dict[str, str]) -> bytes:
    result = _run_local_git(arguments, cwd, env)
    if result.returncode != 0:
        raise AssertionError("synthetic local Git fixture setup failed")
    return result.stdout


class Issue79RegressionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.scanner = _scanner_namespace()
        cls.verification = _verification_module(PACKET_TEXT)

    def inspect(self, code: str) -> str | None:
        return self.scanner["inspect_python_heredoc"](code, False)  # type: ignore[operator]

    def shell_violation(self, command: str) -> str | None:
        self.scanner["shell_owned_path_variables"].clear()  # type: ignore[union-attr]
        self.scanner["shell_pending_owned_bindings"].clear()  # type: ignore[union-attr]
        return self.scanner["forbidden_command"](shlex.split(command))  # type: ignore[operator]

    def test_path_filesystem_readers_require_reviewed_paths(self) -> None:
        unsafe = (
            'from pathlib import Path\nprint(list(Path("synthetic-private").glob("*")))\n',
            'from pathlib import Path\nprint(Path("synthetic-private/file").stat())\n',
            'from pathlib import Path\nprint(list(Path("synthetic-private").iterdir()))\n',
            'from pathlib import Path\nprint(list(Path("synthetic-private").walk()))\n',
            'from pathlib import Path\nreader = Path("synthetic-private").glob\nprint(list(reader("*")))\n',
            'from pathlib import Path\np: Path = Path("synthetic-private")\nprint(p.read_text())\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        safe_bodies = (
            'from pathlib import Path\nprint(Path("docs/evidence/g01-recovery-packet.md").read_text())\n',
            'from pathlib import Path\nprint(Path("docs/evidence/g01-recovery-packet.md").stat())\n',
            'import ast\nlist(ast.walk(ast.parse("value = 1")))\n',
            'import re\nmatch = re.match("x", "x")\nprint(match.group(0))\n',
            'from pathlib import Path\n'
            'def source_fuzz_guard(go_repo_root, module_dir, package_value):\n'
            '    package_dir = (go_repo_root / module_dir / package_value).resolve()\n'
            '    try:\n'
            '        package_dir.relative_to(go_repo_root / module_dir)\n'
            '    except ValueError:\n'
            '        raise SystemExit("outside reviewed package root")\n'
            '    return list(package_dir.glob("*.go"))\n',
        )
        for body in safe_bodies:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_environment_taint_reaches_loop_and_comprehension_targets(self) -> None:
        loop = 'import os\nfor value in os.environ.values():\n    print(value)\n'
        comprehension = 'import os\n[print(value) for value in os.environ.values()]\n'
        self.assertIsNotNone(self.inspect(loop))
        self.assertIsNotNone(self.inspect(comprehension))
        safe = 'for value in ["reviewed"]:\n    print(value)\n'
        self.assertIsNone(self.inspect(safe))

    def test_launcher_aliases_from_iterables_are_rejected(self) -> None:
        direct = (
            'import subprocess\n'
            'for launch in [subprocess.run]:\n'
            '    launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        container = (
            'import subprocess\n'
            'launchers = [subprocess.run]\n'
            'for launch in launchers:\n'
            '    launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        comprehension = (
            'import subprocess\n'
            '[launch(["gh", "workflow", "run", "ci.yml"]) '
            'for launch in [subprocess.run]]\n'
        )
        for body in (direct, container, comprehension):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        safe = 'for transform in [str.upper]:\n    transform("reviewed")\n'
        self.assertIsNone(self.inspect(safe))

    def test_environment_dump_builtins_are_narrowly_allowed(self) -> None:
        for command in ("export", "export -p", "set", "set -o posix"):
            with self.subTest(command=command):
                self.assertIsNotNone(self.shell_violation(command))
        for command in (
            "set -euo pipefail",
            "export PATH=/opt/homebrew/bin:/usr/bin:/bin",
            "export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null",
            "export GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0=core.fsmonitor GIT_CONFIG_VALUE_0=false GIT_CONFIG_KEY_1=core.hooksPath GIT_CONFIG_VALUE_1=/dev/null",
        ):
            with self.subTest(command=command):
                self.assertIsNone(self.shell_violation(command))

    def test_git_child_environment_uses_a_positive_allowlist(self) -> None:
        child_environment = _safe_environment_mapping(self.verification)
        for name in ("GH_TOKEN", "GITHUB_TOKEN", "GITHUB_APP_PRIVATE_KEY", "CUSTOM_SECRET", "HOME"):
            self.assertNotIn(name, child_environment)
        self.assertEqual(child_environment.get("PATH"), "/synthetic/bin")
        self.assertEqual(child_environment.get("LANG"), "C")
        self.assertEqual(child_environment.get("GIT_CONFIG_NOSYSTEM"), "1")
        self.assertEqual(child_environment.get("GIT_CONFIG_GLOBAL"), "/dev/null")

    def test_final_parity_helper_rejects_intent_bits_and_raw_byte_divergence(self) -> None:
        function = _verification_function(self.verification, "require_packet_head_parity")
        verifier_text = PACKET_TEXT[
            PACKET_TEXT.index("The following dynamic command is the live final-verification template."):
        ]
        required_order = (
            'git_query(["rev-parse", "HEAD"])',
            'git_query(["ls-files", "-v", "-z", "--", packet_path.as_posix()])',
            'run_bounded_git_packet_blob_query(\n    f"{local}:{packet_path.as_posix()}"',
            "packet_path.read_bytes()",
            "require_packet_head_parity(\n    intent_result.stdout",
            'git_query(["status", "--porcelain=v1", "--untracked-files=all"])',
        )
        order = [verifier_text.index(item) for item in required_order]
        self.assertEqual(order, sorted(order))
        namespace: dict[str, object] = {
            "__builtins__": __builtins__,
        }
        exec(
            compile(ast.Module(body=[function], type_ignores=[]), "<parity-helper>", "exec"),
            namespace,
        )
        helper = namespace["require_packet_head_parity"]
        with tempfile.TemporaryDirectory(prefix="gh-runnerd-issue79-") as directory:
            root = Path(directory)
            relative = Path("docs/evidence/g01-recovery-packet.md")
            packet = root / relative
            packet.parent.mkdir(parents=True)
            reviewed_bytes = b"synthetic reviewed packet bytes\x00\n"
            changed_bytes = b"synthetic modified packet bytes\x00\n"
            packet.write_bytes(reviewed_bytes)
            env = {
                "PATH": "/usr/bin:/bin",
                "HOME": directory,
                "GIT_CONFIG_NOSYSTEM": "1",
                "GIT_CONFIG_GLOBAL": os.devnull,
                "GIT_CONFIG_SYSTEM": os.devnull,
                "LC_ALL": "C",
            }
            _run_git_checked(["init", "-q"], root, env)
            _run_git_checked(["add", relative.as_posix()], root, env)
            _run_git_checked(
                ["-c", "user.name=synthetic", "-c", "user.email=synthetic@example.invalid", "commit", "-q", "-m", "baseline"],
                root,
                env,
            )
            head = _run_git_checked(["rev-parse", "HEAD"], root, env).decode().strip()
            intent = _run_git_checked(["ls-files", "-v", "-z", "--", relative.as_posix()], root, env)
            blob = _run_git_checked(["show", f"{head}:{relative.as_posix()}"], root, env)
            helper(intent, blob, packet.read_bytes())

            for flag, clear_flag in (
                ("--skip-worktree", "--no-skip-worktree"),
                ("--assume-unchanged", "--no-assume-unchanged"),
            ):
                _run_git_checked(["update-index", flag, relative.as_posix()], root, env)
                packet.write_bytes(changed_bytes)
                legacy_status = _run_git_checked(
                    ["status", "--porcelain=v1", "--untracked-files=all"], root, env
                )
                self.assertEqual(legacy_status, b"")
                with self.assertRaises(SystemExit):
                    helper(
                        _run_git_checked(
                            ["ls-files", "-v", "-z", "--", relative.as_posix()], root, env
                        ),
                        _run_git_checked(["show", f"{head}:{relative.as_posix()}"], root, env),
                        packet.read_bytes(),
                    )
                _run_git_checked(["update-index", clear_flag, relative.as_posix()], root, env)
                packet.write_bytes(reviewed_bytes)

            packet.write_bytes(changed_bytes)
            with self.assertRaises(SystemExit):
                helper(
                    _run_git_checked(
                        ["ls-files", "-v", "-z", "--", relative.as_posix()], root, env
                    ),
                    _run_git_checked(["show", f"{head}:{relative.as_posix()}"], root, env),
                    packet.read_bytes(),
                )

    def test_large_packet_blob_uses_a_separate_bounded_capture(self) -> None:
        runtime = _bounded_git_query_namespace(self.verification)
        with tempfile.TemporaryDirectory(prefix="gh-runnerd-issue79-blob-") as directory:
            root = Path(directory)
            relative = Path("docs/evidence/g01-recovery-packet.md")
            packet = root / relative
            packet.parent.mkdir(parents=True)
            payload_size = max(len(PACKET_TEXT.encode("utf-8")), 64 * 1024 + 1)
            payload = b"S" * payload_size
            packet.write_bytes(payload)
            env = {
                "PATH": "/usr/bin:/bin",
                "HOME": directory,
                "GIT_CONFIG_NOSYSTEM": "1",
                "GIT_CONFIG_GLOBAL": os.devnull,
                "GIT_CONFIG_SYSTEM": os.devnull,
                "GIT_ATTR_NOSYSTEM": "1",
                "LC_ALL": "C",
            }
            _run_git_checked(["init", "-q"], root, env)
            _run_git_checked(["add", relative.as_posix()], root, env)
            _run_git_checked(
                [
                    "-c", "user.name=synthetic",
                    "-c", "user.email=synthetic@example.invalid",
                    "commit", "-q", "-m", "baseline",
                ],
                root,
                env,
            )
            blob_spec = f"HEAD:{relative.as_posix()}"
            blob_query = runtime.get("run_bounded_git_packet_blob_query")
            if not callable(blob_query):
                result = runtime["run_bounded_git_query"](
                    runtime["git_query"](["show", blob_spec]), cwd=root, env=env
                )
            else:
                result = blob_query(blob_spec, cwd=root, env=env)
            self.assertEqual(result.stdout, payload)
            self.assertEqual(runtime["git_query_output_max_bytes"], 64 * 1024)
            self.assertEqual(runtime["git_query_packet_blob_output_max_bytes"], 8 * 1024 * 1024)
            with self.assertRaises(SystemExit):
                blob_query("HEAD:README.md", cwd=root, env=env)
            with self.assertRaisesRegex(SystemExit, "output exceeded the reviewed budget"):
                runtime["run_bounded_git_query"](
                    runtime["git_query"](["show", blob_spec]), cwd=root, env=env
                )

    def test_git_config_include_options_are_rejected_before_read_only_classification(self) -> None:
        for command in (
            "git -c include.path=synthetic/included.cfg status",
            "git -c includeIf.gitdir:/synthetic/repo.path=synthetic/included.cfg status",
            "git -cinclude.path=synthetic/included.cfg status",
            "git --config-env=include.path=SYNTHETIC_INCLUDE status",
        ):
            with self.subTest(command=command):
                self.assertIsNotNone(self.shell_violation(command))
        for command in ("git -P status", "git -c core.fsmonitor=false status"):
            with self.subTest(command=command):
                self.assertIsNone(self.shell_violation(command))


if __name__ == "__main__":
    unittest.main(verbosity=2)
