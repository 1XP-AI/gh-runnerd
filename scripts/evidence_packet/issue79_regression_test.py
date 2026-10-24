"""Offline, non-executing regression probes for issue #79 review findings.

Python examples and shell commands supplied to the packet scanner remain data:
the harness parses/inspects them but never evaluates or launches them. Child
processes are limited to literal Git commands against temporary local
repositories and an isolated ``sys.executable -I -B -c`` probe that verifies a
synthetic local module cannot shadow a standard-library import.
"""

from __future__ import annotations

import ast
import builtins
import os
import re
import shlex
import selectors
import signal
import subprocess
import sys
import tempfile
import time
import types
import unittest
from unittest import mock
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
    verification_start = source.find("\nmatches = []\n", source.index(
        "def inspect_python_heredoc(body, safe_marker):"
    ))
    if verification_start < 0:
        raise AssertionError("packet scanner verification boundary is missing")
    return source[:verification_start]


def _literal_definition_time_expression(node: ast.AST) -> bool:
    try:
        ast.literal_eval(node)
    except (ValueError, TypeError, SyntaxError, RecursionError):
        return False
    return True


def _validated_scanner_statements(module: ast.Module) -> tuple[ast.stmt, ...]:
    """Validate packet-controlled imports and definition-time expressions before exec."""
    top_level = {id(statement) for statement in module.body}
    allowed_imports = {"ast", "re", "shlex", "subprocess"}
    protected_names = set(dir(builtins)) | {
        "Path", "ast", "os", "re", "shlex", "source", "subprocess",
        "tempfile", "types", "selectors", "signal", "time", "__builtins__",
    }
    for node in ast.walk(module):
        if isinstance(node, ast.Import):
            if id(node) not in top_level or any(
                alias.name not in allowed_imports or alias.asname is not None
                for alias in node.names
            ):
                raise AssertionError("packet scanner import is not explicitly reviewed")
        elif isinstance(node, ast.ImportFrom):
            if (
                id(node) not in top_level
                or node.level != 0
                or node.module != "pathlib"
                or len(node.names) != 1
                or node.names[0].name != "Path"
                or node.names[0].asname is not None
            ):
                raise AssertionError("packet scanner from-import is not explicitly reviewed")
        elif isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            argument_annotations = [
                argument.annotation
                for argument in (
                    list(node.args.posonlyargs)
                    + list(node.args.args)
                    + list(node.args.kwonlyargs)
                )
            ]
            if node.args.vararg is not None:
                argument_annotations.append(node.args.vararg.annotation)
            if node.args.kwarg is not None:
                argument_annotations.append(node.args.kwarg.annotation)
            if (
                node.decorator_list
                or node.returns is not None
                or getattr(node, "type_params", ())
                or any(annotation is not None for annotation in argument_annotations)
                or not all(
                    _literal_definition_time_expression(default)
                    for default in node.args.defaults
                )
                or not all(
                    default is None or _literal_definition_time_expression(default)
                    for default in node.args.kw_defaults
                )
            ):
                raise AssertionError(
                    "packet scanner function has unreviewed definition-time expressions"
                )
            if id(node) in top_level and node.name in protected_names:
                raise AssertionError("packet scanner function shadows a protected binding")
        elif isinstance(node, ast.Lambda) and not all(
            _literal_definition_time_expression(default)
            for default in node.args.defaults
        ):
            raise AssertionError("packet scanner lambda has an unreviewed default expression")
        elif isinstance(node, (ast.Assign, ast.AnnAssign)) and id(node) in top_level:
            targets = node.targets if isinstance(node, ast.Assign) else [node.target]
            if any(not isinstance(target, ast.Name) for target in targets):
                raise AssertionError("packet scanner assignment target is not a simple name")
            bound_names = {target.id for target in targets}
            if bound_names & (protected_names - {"source"}):
                raise AssertionError("packet scanner assignment shadows a protected binding")
    if any(not isinstance(statement, (
        ast.Import, ast.ImportFrom, ast.FunctionDef, ast.AsyncFunctionDef,
        ast.Assign, ast.AnnAssign,
    )) for statement in module.body):
        raise AssertionError("packet scanner has an unsupported top-level statement")
    return tuple(module.body)


def _require_scanner_ast_runtime() -> None:
    required = ("Match", "MatchAs", "MatchStar", "MatchMapping", "TryStar")
    missing = [name for name in required if not isinstance(getattr(ast, name, None), type)]
    if missing:
        raise RuntimeError(
            "scanner regression harness requires Python 3.11+ AST support; missing "
            + ", ".join(missing)
        )


def _scanner_namespace() -> dict[str, object]:
    _require_scanner_ast_runtime()
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
    validated_statements = _validated_scanner_statements(module)
    for statement in validated_statements:
        if isinstance(statement, (ast.Import, ast.ImportFrom)):
            # Names used by the scanner are preloaded above; packet text never
            # gets to select or execute an import during harness setup.
            continue
        elif isinstance(statement, (ast.FunctionDef, ast.AsyncFunctionDef)):
            exec(compile(ast.Module(body=[statement], type_ignores=[]), "<packet-scanner-function>", "exec"), namespace)
        elif isinstance(statement, (ast.Assign, ast.AnnAssign)):
            if isinstance(statement, ast.Assign):
                names = set().union(*(_target_names(target) for target in statement.targets))
                value = statement.value
            else:
                names = _target_names(statement.target)
                value = statement.value
            if "source" in names:
                reviewed_source = ast.parse(
                    'Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")',
                    mode="eval",
                ).body
                if names != {"source"} or value is None or ast.dump(
                    value, include_attributes=False
                ) != ast.dump(reviewed_source, include_attributes=False):
                    raise AssertionError(
                        "packet scanner source assignment is not reviewed"
                    )
                continue
            if value is None:
                continue
            if _safe_assignment_expression(value, namespace):
                safe_statement = statement
                if isinstance(statement, ast.AnnAssign):
                    safe_statement = ast.Assign(
                        targets=[statement.target],
                        value=statement.value,
                    )
                    ast.copy_location(safe_statement, statement)
                exec(compile(ast.Module(body=[safe_statement], type_ignores=[]), "<packet-scanner-constant>", "exec"), namespace)
            else:
                raise AssertionError(
                    "packet scanner has an unsupported top-level assignment"
                )
        else:
            raise AssertionError("packet scanner has an unsupported top-level statement")
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
    matches = [
        statement
        for statement in module.body
        if (
            isinstance(statement, ast.Assign)
            and any(
                isinstance(target, ast.Name) and target.id == name
                for target in statement.targets
            )
        )
        or (
            isinstance(statement, ast.AnnAssign)
            and isinstance(statement.target, ast.Name)
            and statement.target.id == name
        )
    ]
    if len(matches) != 1:
        raise AssertionError(
            f"verification template assignment {name!r} is missing or duplicated"
        )
    return matches[0]


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
    matches = [
        statement
        for statement in ast.walk(module)
        if isinstance(statement, (ast.FunctionDef, ast.AsyncFunctionDef))
        and statement.name == name
    ]
    if (
        len(matches) == 1
        and isinstance(matches[0], ast.FunctionDef)
        and matches[0] in module.body
    ):
        return matches[0]
    raise AssertionError(f"verification helper {name!r} is missing or duplicated")


def _safe_integer_expression(node: ast.AST) -> int:
    if isinstance(node, ast.Constant) and type(node.value) is int:
        return node.value
    if isinstance(node, ast.BinOp) and isinstance(node.op, ast.Mult):
        return _safe_integer_expression(node.left) * _safe_integer_expression(node.right)
    if isinstance(node, ast.BinOp) and isinstance(node.op, ast.Add):
        return _safe_integer_expression(node.left) + _safe_integer_expression(node.right)
    raise AssertionError("Git query budget/deadline is not a literal integer expression")


def _validate_packet_function_definition(
    node: ast.FunctionDef,
    expected_name: str,
    allowed_default_names: set[str] | None = None,
) -> None:
    """Reject packet-controlled definition-time expressions before compilation."""
    allowed_default_names = allowed_default_names or set()

    def reviewed_default(expression: ast.AST) -> bool:
        return _literal_definition_time_expression(expression) or (
            isinstance(expression, ast.Name)
            and expression.id in allowed_default_names
        )

    argument_annotations = [
        argument.annotation
        for argument in (
            list(node.args.posonlyargs)
            + list(node.args.args)
            + list(node.args.kwonlyargs)
        )
    ]
    if node.args.vararg is not None:
        argument_annotations.append(node.args.vararg.annotation)
    if node.args.kwarg is not None:
        argument_annotations.append(node.args.kwarg.annotation)
    if (
        node.name != expected_name
        or node.decorator_list
        or node.returns is not None
        or getattr(node, "type_params", ())
        or any(annotation is not None for annotation in argument_annotations)
        or not all(
            reviewed_default(default)
            for default in node.args.defaults
        )
        or not all(
            default is None or reviewed_default(default)
            for default in node.args.kw_defaults
        )
    ):
        raise AssertionError(
            "packet scanner function has unreviewed definition-time expressions"
        )


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
            allowed_defaults = (
                {"git_query_output_max_bytes"}
                if statement.name == "run_bounded_git_query"
                else set()
            )
            _validate_packet_function_definition(
                statement, statement.name, allowed_defaults
            )
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


def _isolated_git_shell_command(arguments: list[str]) -> str:
    """Build inert scanner data for the explicit standalone Git boundary."""
    return shlex.join(
        [
            "/usr/bin/env", "-i",
            "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
            "GIT_CONFIG_SYSTEM=/dev/null", "GIT_ATTR_NOSYSTEM=1",
            "/usr/bin/git", "--no-replace-objects", "-P",
            "-c", "core.fsmonitor=false",
            "-c", "core.hooksPath=/dev/null",
            *arguments,
        ]
    )


class ScannerRuntimeTests(unittest.TestCase):
    def test_missing_ast_support_refuses_before_loading_scanner(self) -> None:
        for name in ("Match", "MatchAs", "MatchStar", "MatchMapping", "TryStar"):
            with self.subTest(feature=name), mock.patch.object(ast, name, None), mock.patch(
                __name__ + "._scanner_module_source",
                side_effect=AssertionError("scanner definitions must not load"),
            ) as source:
                with self.assertRaisesRegex(RuntimeError, "Python 3.11"):
                    _scanner_namespace()
                source.assert_not_called()

    def test_supported_ast_runtime_keeps_loader_available(self) -> None:
        with mock.patch(__name__ + "._scanner_module_source", return_value="") as source:
            namespace = _scanner_namespace()
        source.assert_called_once_with(PACKET_TEXT)
        self.assertIs(namespace["ast"], ast)


class Issue79RegressionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.scanner = _scanner_namespace()
        cls.verification = _verification_module(PACKET_TEXT)

    def inspect(self, code: str) -> str | None:
        return self.scanner["inspect_python_heredoc"](code, False)  # type: ignore[operator]

    def packet_heredoc_containing(self, *fragments: str) -> tuple[str, bool]:
        matches = [
            (body, marker)
            for _number, body, marker, _invocation in self.scanner[
                "python_heredoc_bodies"
            ](PACKET_TEXT)  # type: ignore[operator]
            if all(fragment in body for fragment in fragments)
        ]
        self.assertEqual(1, len(matches), f"expected one packet heredoc containing {fragments!r}")
        return matches[0]

    def markdown_link_target_path_is_reviewed(self, code: str) -> bool:
        tree = ast.parse(code, filename="<markdown-link-check-specimen>")
        parents = {
            child: parent
            for parent in ast.walk(tree)
            for child in ast.iter_child_nodes(parent)
        }
        path = next(
            node
            for node in ast.walk(tree)
            if isinstance(node, ast.Name)
            and node.id == "path"
            and isinstance(parents.get(node), ast.Attribute)
            and parents[node].attr == "is_file"
        )
        return self.scanner["python_reviewed_markdown_link_target_path"](
            path, tree, parents
        )  # type: ignore[operator]

    def test_markdown_link_containment_guard_must_be_direct_and_reachable(self) -> None:
        canonical = (
            'import subprocess\n'
            'from pathlib import Path\n'
            'git_query_environment = {"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null", "GIT_ATTR_NOSYSTEM": "1", "GIT_CONFIG_COUNT": "2", "GIT_CONFIG_KEY_0": "core.fsmonitor", "GIT_CONFIG_VALUE_0": "false", "GIT_CONFIG_KEY_1": "core.hooksPath", "GIT_CONFIG_VALUE_1": "/dev/null"}\n'
            'def git_command(arguments):\n'
            '    return ["/usr/bin/env", "-i", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_ATTR_NOSYSTEM=1", "/usr/bin/git", "--no-replace-objects", "-P", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", *arguments]\n'
            'files = subprocess.check_output(git_command(["ls-files", "--", "*.md"]), env=git_query_environment, text=True).splitlines()\n'
            'repository_root = Path.cwd().resolve()\n'
            'for name in files:\n'
            '    source = Path(name)\n'
            '    if source.is_absolute() or ".." in source.parts:\n'
            '        continue\n'
            '    if source.is_symlink():\n'
            '        continue\n'
            '    source_path = (repository_root / source).resolve()\n'
            '    try:\n'
            '        source_path.relative_to(repository_root)\n'
            '    except ValueError:\n'
            '        continue\n'
            '    markdown = source_path.read_text(encoding="utf-8")\n'
            '    for match in link.finditer(markdown):\n'
            '        target = match.group(1).strip().strip("<>")\n'
            '        if target.startswith("#"):\n'
            '            fragment = target[1:]\n'
            '            target = source.name\n'
            '        else:\n'
            '            target, separator, fragment = target.partition("#")\n'
            '            fragment = fragment if separator else None\n'
            '        path = (source.parent / target).resolve()\n'
            '        try:\n'
            '            path.relative_to(repository_root)\n'
            '        except ValueError:\n'
            '            continue\n'
            '        if not path.is_file():\n'
            '            errors.append(target)\n'
        )
        nested_relative_to = canonical.replace(
            '            path.relative_to(repository_root)\n',
            '            if False:\n'
            '                path.relative_to(repository_root)\n',
            1,
        )
        unreachable_try = canonical.replace(
            '        try:\n'
            '            path.relative_to(repository_root)\n'
            '        except ValueError:\n'
            '            continue\n',
            '        if False:\n'
            '            try:\n'
            '                path.relative_to(repository_root)\n'
            '            except ValueError:\n'
            '                continue\n',
            1,
        )
        path_rebound_after_guard = canonical.replace(
            '        if not path.is_file():\n',
            '        path = Path("synthetic-private/file")\n'
            '        if not path.is_file():\n',
            1,
        )
        guard_before_approved_path_assignment = canonical.replace(
            '        path = (source.parent / target).resolve()\n'
            '        try:\n'
            '            path.relative_to(repository_root)\n'
            '        except ValueError:\n'
            '            continue\n',
            '        try:\n'
            '            path.relative_to(repository_root)\n'
            '        except ValueError:\n'
            '            continue\n'
            '        path = (source.parent / target).resolve()\n',
            1,
        )
        self.assertNotEqual(canonical, nested_relative_to)
        self.assertNotEqual(canonical, unreachable_try)
        self.assertNotEqual(canonical, path_rebound_after_guard)
        self.assertNotEqual(canonical, guard_before_approved_path_assignment)
        self.assertTrue(self.markdown_link_target_path_is_reviewed(canonical))
        self.assertFalse(self.markdown_link_target_path_is_reviewed(nested_relative_to))
        self.assertFalse(self.markdown_link_target_path_is_reviewed(unreachable_try))
        for specimen in (
            path_rebound_after_guard,
            guard_before_approved_path_assignment,
        ):
            with self.subTest(specimen=specimen):
                self.assertFalse(self.markdown_link_target_path_is_reviewed(specimen))

    def test_regex_group_exemption_respects_shadowing_parameters(self) -> None:
        shadowed = (
            'import re\n'
            'match = re.match("x", "x")\n'
            'def render(match):\n'
            '    print(match.group())\n'
        )
        canonical = (
            'import re\n'
            'match = re.match("x", "x")\n'
            'print(match.group(0))\n'
        )
        self.assertIsNotNone(self.inspect(shadowed))
        self.assertIsNone(self.inspect(canonical))

    def test_sensitive_return_through_factory_created_instance_is_tainted(self) -> None:
        unsafe = (
            'import os\n'
            'class Snapshot:\n'
            '    def read(self):\n'
            '        return dict(os.environ)\n'
            'def build():\n'
            '    instance = Snapshot()\n'
            '    return instance\n'
            'print(build().read())\n'
        )
        safe = (
            'class Snapshot:\n'
            '    def read(self):\n'
            '        return {"status": "ready"}\n'
            'def build():\n'
            '    instance = Snapshot()\n'
            '    return instance\n'
            'print(build().read())\n'
        )
        self.assertIsNotNone(self.inspect(unsafe))
        self.assertIsNone(self.inspect(safe))

    def test_getattr_default_bound_method_taints_sensitive_arguments(self) -> None:
        unsafe = (
            'import os\n'
            'class Sink:\n'
            '    def emit(self, value):\n'
            '        print(value)\n'
            'sink = Sink()\n'
            'callback = getattr(sink, "missing", sink.emit)\n'
            'callback(os.environ)\n'
        )
        safe = (
            'class Sink:\n'
            '    def emit(self, value):\n'
            '        print(value)\n'
            'sink = Sink()\n'
            'callback = getattr(sink, "missing", sink.emit)\n'
            'callback({"status": "ready"})\n'
        )
        self.assertIsNotNone(self.inspect(unsafe))
        self.assertIsNone(self.inspect(safe))

    def package_directory_guard_is_reviewed(self, code: str) -> bool:
        tree = ast.parse(code, filename="<package-containment-specimen>")
        parents = {
            child: parent
            for parent in ast.walk(tree)
            for child in ast.iter_child_nodes(parent)
        }
        package_dir = next(
            node
            for node in ast.walk(tree)
            if isinstance(node, ast.Name)
            and node.id == "package_dir"
            and isinstance(node.ctx, ast.Load)
            and isinstance(parents.get(node), ast.Attribute)
            and parents[node].attr == "glob"
        )
        return self.scanner["python_reviewed_go_package_directory"](
            package_dir, tree, parents
        )  # type: ignore[operator]

    def replace_source_fuzz_guard_fragment(
        self, code: str, original: str, replacement: str
    ) -> str:
        tree = ast.parse(code, filename="<package-containment-specimen>")
        guards = [
            statement
            for statement in tree.body
            if isinstance(statement, ast.FunctionDef)
            and statement.name == "source_fuzz_guard"
        ]
        self.assertEqual(len(guards), 1)
        source_lines = code.splitlines(keepends=True)
        start = sum(len(line) for line in source_lines[: guards[0].lineno - 1])
        end = sum(len(line) for line in source_lines[: guards[0].end_lineno])
        guard_source = code[start:end]
        changed_guard = guard_source.replace(original, replacement, 1)
        self.assertNotEqual(changed_guard, guard_source)
        return code[:start] + changed_guard + code[end:]

    def shell_violation(self, command: str) -> str | None:
        self.scanner["shell_owned_path_variables"].clear()  # type: ignore[union-attr]
        self.scanner["shell_pending_owned_bindings"].clear()  # type: ignore[union-attr]
        return self.scanner["forbidden_shell_command"](shlex.split(command))  # type: ignore[operator]

    def shell_document_violation(self, commands: str) -> str | None:
        """Use the packet's shell-fence parser and scanner on inert source text."""
        markdown = f"```sh\n{commands}\n```\n"
        for command, _number in self.scanner["shell_commands"](markdown):  # type: ignore[operator]
            for segment in self.scanner["shell_token_segments"](command):  # type: ignore[operator]
                violation = self.scanner["forbidden_shell_command"](segment)  # type: ignore[operator]
                if violation:
                    return violation
        return None

    def test_path_filesystem_readers_require_reviewed_paths(self) -> None:
        unsafe = (
            'from pathlib import Path\nprint(list(Path("synthetic-private").glob("*")))\n',
            'from pathlib import Path\nprint(Path("synthetic-private/file").stat())\n',
            'from pathlib import Path\nprint(list(Path("synthetic-private").iterdir()))\n',
            'from pathlib import Path\nprint(list(Path("synthetic-private").walk()))\n',
            'from pathlib import Path\nreader = Path("synthetic-private").glob\nprint(list(reader("*")))\n',
            'from pathlib import Path\np: Path = Path("synthetic-private")\nprint(p.read_text())\n',
            'from pathlib import Path\nfactory = Path\nprint(factory("synthetic-private").read_text())\n',
            'from pathlib import Path\nprint(Path("synthetic-private").resolve().read_text())\n',
            'from pathlib import Path\ndef path_factory():\n    return Path("synthetic-private")\nprint(path_factory().read_text())\n',
            'from pathlib import Path\n'
            'def reader_factory():\n'
            '    return Path("synthetic-private/file").read_text\n'
            'reader = reader_factory()\n'
            'print(reader())\n',
            'from pathlib import Path\n'
            'def reader_factory(flag):\n'
            '    if flag:\n'
            '        return Path("synthetic-private/file").read_text\n'
            '    return Path("docs/evidence/g01-recovery-packet.md").read_text\n'
            'reader = reader_factory(True)\n'
            'print(reader())\n',
            'from pathlib import Path\n'
            'member = "read_text"\n'
            'def reader_factory():\n'
            '    return getattr(Path("synthetic-private/file"), member)\n'
            'reader = reader_factory()\n'
            'print(reader())\n',
            'from pathlib import Path\ndef read_private(path: Path):\n    return path.read_text()\n',
            'import ast\nfrom pathlib import Path\nast = Path("synthetic-private")\nprint(list(ast.walk()))\n',
            'import re\nfrom pathlib import Path\nmatch = re.match("a", "a")\nmatch = Path("synthetic-private")\nprint(match.group())\n',
            'from pathlib import Path\n'
            'def source_fuzz_guard(go_repo_root, module_dir, package_value):\n'
            '    package_dir = (go_repo_root / module_dir / package_value).resolve()\n'
            '    try:\n'
            '        package_dir.relative_to(go_repo_root / module_dir)\n'
            '    except ValueError:\n'
            '        raise SystemExit("outside caller roots")\n'
            '    print(list(package_dir.glob("*")))\n'
            'source_fuzz_guard(Path("/"), Path("etc"), Path(""))\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        safe_bodies = (
            'from pathlib import Path\nprint(Path("docs/evidence/g01-recovery-packet.md").read_text())\n',
            'from pathlib import Path\n'
            'def reviewed_reader_factory():\n'
            '    return Path("docs/evidence/g01-recovery-packet.md").read_text\n'
            'reader = reviewed_reader_factory()\n'
            'print(reader())\n',
            'from pathlib import Path\n'
            'member = "read_text"\n'
            'def reviewed_reader_factory():\n'
            '    return getattr(Path("docs/evidence/g01-recovery-packet.md"), member)\n'
            'reader = reviewed_reader_factory()\n'
            'print(reader())\n',
            'from pathlib import Path\n'
            'source = Path("scripts/evidence_packet/issue79_regression_test.py").read_bytes()\n'
            'if not source:\n    raise SystemExit("reviewed source is empty")\n',
            'from pathlib import Path\nprint(Path("docs/evidence/g01-recovery-packet.md").stat())\n',
            'import ast\nlist(ast.walk(ast.parse("value = 1")))\n',
            'import re\nmatch = re.match("x", "x")\nprint(match.group(0))\n',
        )
        for body in safe_bodies:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_resolved_local_paths_are_not_disclosed_to_output_sinks(self) -> None:
        unsafe = (
            'from pathlib import Path\nprint(Path.cwd().resolve())\n',
            'from pathlib import Path\nresolved = Path("/synthetic/worktree").resolve()\n'
            'print(f"root={resolved}")\n',
            'from pathlib import Path\n'
            'value = format(Path.cwd().resolve())\n'
            'print(value)\n',
            'from pathlib import Path\n'
            'value = ascii(Path.cwd().resolve())\n'
            'print(value)\n',
            'from pathlib import Path\n'
            'value = Path.cwd().resolve().__str__()\n'
            'print(value)\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        internal_use = (
            'from pathlib import Path\nresolved = Path.cwd().resolve()\n'
            'if not resolved.is_absolute():\n    raise SystemExit("invalid root")\n'
        )
        self.assertIsNone(self.inspect(internal_use))
        safe_formatting = (
            'print(format("reviewed"))\n'
            'print(ascii("reviewed"))\n'
            'print("reviewed".__str__())\n'
        )
        self.assertIsNone(self.inspect(safe_formatting))

    def test_resolved_paths_keep_taint_through_protocol_and_byte_conversions(self) -> None:
        unsafe = (
            'from pathlib import Path\n'
            'convert = ascii\n'
            'print(convert(Path.cwd().resolve()))\n',
            'from pathlib import Path\n'
            'print(Path.cwd().resolve().__fspath__())\n',
            'from pathlib import Path\n'
            'print(Path.cwd().resolve().as_posix().encode().decode())\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'from pathlib import Path\n'
            'convert = ascii\n'
            'print(convert(Path("docs/evidence/g01-recovery-packet.md")))\n',
            'from pathlib import Path\n'
            'print(Path("docs/evidence/g01-recovery-packet.md").__fspath__())\n',
            'from pathlib import Path\n'
            'print(Path("docs/evidence/g01-recovery-packet.md").as_posix().encode().decode())\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_concatenated_getattr_path_reader_is_rejected(self) -> None:
        unsafe = (
            'from pathlib import Path\n'
            'member = "read_" + "text"\n'
            'reader = getattr(Path("synthetic-private/file"), member)\n'
            'print(reader())\n',
            'from pathlib import Path\n'
            'member = "read_" + suffix\n'
            'reader = getattr(Path("synthetic-private/file"), member)\n'
            'print(reader())\n',
            'from pathlib import Path\n'
            'lookup = getattr\n'
            'member = "read_" + suffix\n'
            'reader = lookup(Path("synthetic-private/file"), member)\n'
            'print(reader())\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'from pathlib import Path\n'
            'member = "read_" + "text"\n'
            'reader = getattr(Path("docs/evidence/g01-recovery-packet.md"), member)\n'
            'print(reader())\n',
            'from pathlib import Path\n'
            'lookup = getattr\n'
            'member = "read_" + "text"\n'
            'reader = lookup(Path("docs/evidence/g01-recovery-packet.md"), member)\n'
            'print(reader())\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_nested_function_name_collision_does_not_hide_launcher_alias(self) -> None:
        body = (
            'import subprocess\n'
            'def launcher_factory():\n'
            '    return subprocess.run\n'
            'def unrelated_scope():\n'
            '    def launcher_factory():\n'
            '        return len\n'
            'launch = launcher_factory()\n'
            'launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        self.assertIsNotNone(self.inspect(body))

        safe = (
            'def value_factory():\n'
            '    return len\n'
            'def unrelated_scope():\n'
            '    def value_factory():\n'
            '        return str.upper\n'
            'value = value_factory()\n'
            'value("reviewed")\n'
        )
        self.assertIsNone(self.inspect(safe))

    def test_sensitive_local_helper_returns_are_tainted_at_output_sinks(self) -> None:
        unsafe = (
            'import os\n'
            'def environment_snapshot():\n'
            '    return os.environ\n'
            'print(environment_snapshot())\n',
            'import os\n'
            'def environment_snapshot():\n'
            '    return dict(os.environ)\n'
            'print(environment_snapshot())\n',
            'import os\n'
            'def environment_snapshot():\n'
            '    return dict(os.environ)\n'
            'def forwarded_snapshot():\n'
            '    return environment_snapshot()\n'
            'print(forwarded_snapshot())\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'def reviewed_status():\n'
            '    return {"status": "reviewed"}\n'
            'print(reviewed_status())\n'
        )
        self.assertIsNone(self.inspect(safe))

    def test_sensitive_method_and_lambda_returns_are_tainted(self) -> None:
        unsafe = (
            'import os\n'
            'snapshot = lambda: dict(os.environ)\n'
            'print(snapshot())\n',
            'import os\n'
            'class EnvironmentSnapshot:\n'
            '    def read(self):\n'
            '        return dict(os.environ)\n'
            'print(EnvironmentSnapshot().read())\n',
            'import os\n'
            'class EnvironmentSnapshot:\n'
            '    def read(self):\n'
            '        return dict(os.environ)\n'
            'snapshot = EnvironmentSnapshot()\n'
            'print(snapshot.read())\n',
            'import os\n'
            'class EnvironmentSnapshot:\n'
            '    def read(self):\n'
            '        return dict(os.environ)\n'
            'snapshot = EnvironmentSnapshot()\n'
            'alias = snapshot\n'
            'print(alias.read())\n',
            'import os\n'
            'class EnvironmentSnapshot:\n'
            '    def __init__(self, label):\n'
            '        self.label = label\n'
            '    def read(self):\n'
            '        return dict(os.environ)\n'
            'snapshot = EnvironmentSnapshot("reviewed")\n'
            'print(snapshot.read())\n',
            'import os\n'
            'class EnvironmentSnapshot:\n'
            '    def read(self):\n'
            '        return dict(os.environ)\n'
            'snapshot = EnvironmentSnapshot()\n'
            'reader = snapshot.read\n'
            'print(reader())\n',
            'import os\n'
            'class EnvironmentSnapshot:\n'
            '    def read(self):\n'
            '        return dict(os.environ)\n'
            'def build():\n'
            '    return EnvironmentSnapshot()\n'
            'snapshot = build()\n'
            'print(snapshot.read())\n',
            'import os\n'
            'reader = getattr(object(), "missing", lambda: dict(os.environ))\n'
            'print(reader())\n',
            'import os\n'
            'class Snapshot:\n'
            '    def read(self):\n'
            '        return dict(os.environ)\n'
            'Alias = Snapshot\n'
            'print(Alias().read())\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'class StatusSnapshot:\n'
            '    def read(self):\n'
            '        return {"status": "reviewed"}\n'
            'print(StatusSnapshot().read())\n'
        )
        self.assertIsNone(self.inspect(safe))
        safe_alias = (
            'class StatusSnapshot:\n'
            '    def read(self):\n'
            '        return {"status": "reviewed"}\n'
            'Alias = StatusSnapshot\n'
            'print(Alias().read())\n'
        )
        self.assertIsNone(self.inspect(safe_alias))

    def test_sensitive_values_are_tainted_into_method_and_lambda_parameters(self) -> None:
        unsafe = (
            'import os\n'
            'class C:\n'
            '    def emit(self, payload):\n'
            '        print(payload)\n'
            'C().emit(os.environ)\n',
            'import os\n'
            'emit = lambda payload: print(payload)\n'
            'emit(os.environ)\n',
            'import os\n'
            'class C:\n'
            '    def emit(self, payload):\n'
            '        print(payload)\n'
            'sink = C()\n'
            'member = "emit"\n'
            'callback = getattr(sink, member)\n'
            'callback(os.environ)\n',
            'import os\n'
            'class C:\n'
            '    def emit(self, payload):\n'
            '        print(payload)\n'
            'def build():\n'
            '    instance = C()\n'
            '    return instance\n'
            'sink = build()\n'
            'sink.emit(os.environ)\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'class C:\n'
            '    def emit(self, payload):\n'
            '        print(payload)\n'
            'C().emit({"status": "reviewed"})\n',
            'emit = lambda payload: print(payload)\n'
            'emit({"status": "reviewed"})\n',
            'class C:\n'
            '    def emit(self, payload):\n'
            '        print(payload)\n'
            'sink = C()\n'
            'member = "emit"\n'
            'callback = getattr(sink, member)\n'
            'callback({"status": "reviewed"})\n',
            'class C:\n'
            '    def emit(self, payload):\n'
            '        print(payload)\n'
            'def build():\n'
            '    instance = C()\n'
            '    return instance\n'
            'sink = build()\n'
            'sink.emit({"status": "reviewed"})\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_join_of_environment_views_keeps_sensitive_taint(self) -> None:
        unsafe = (
            'import os\n'
            'secret = os.environ\n'
            "print(''.join(secret.values()))\n",
            'import os\n'
            "print(''.join(os.environ.values()))\n",
            'import os\n'
            'secret = os.environ\n'
            "print(''.join(list(secret.values())))\n",
            'import os\n'
            'secret = os.environ\n'
            "print(''.join(tuple(secret.values())))\n",
            'import os\n'
            'secret = os.environ\n'
            "print(''.join(value for value in secret.values()))\n",
            'import os\n'
            'secret = os.environ\n'
            "print(''.join(map(str, secret.values())))\n",
            'import os\n'
            'secret = os.environ\n'
            'separator = ""\n'
            'print(separator.join(list(secret.values())))\n',
            'import os\n'
            'secret = os.environ\n'
            'join = "".join\n'
            'print(join(list(secret.values())))\n',
            'import os\n'
            'secret = os.environ\n'
            'print(str().join(iter(secret.values())))\n',
            'import os\n'
            'secret = os.environ\n'
            'values = secret.values()\n'
            'print("".join(values))\n',
            'import os\n'
            'secret = os.environ\n'
            'values = list(secret.values())\n'
            'join = "".join\n'
            'print(join(values))\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'snapshot = {"status": "reviewed"}\n'
            "print(''.join(snapshot.values()))\n",
            "print(''.join({'status': 'reviewed'}.values()))\n",
            'snapshot = {"status": "reviewed"}\n'
            "print(''.join(list(snapshot.values())))\n",
            "print(''.join(tuple({'status': 'reviewed'}.values())))\n",
            "print(''.join(value for value in {'status': 'reviewed'}.values()))\n",
            "print(''.join(map(str, {'status': 'reviewed'}.values())))\n",
            'snapshot = {"status": "reviewed"}\n'
            'separator = ""\n'
            'print(separator.join(list(snapshot.values())))\n',
            'snapshot = {"status": "reviewed"}\n'
            'join = "".join\n'
            'print(join(list(snapshot.values())))\n',
            'snapshot = {"status": "reviewed"}\n'
            'values = snapshot.values()\n'
            'print("".join(values))\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_environment_urlencode_output_keeps_sensitive_taint(self) -> None:
        unsafe = (
            'import os\n'
            'import urllib.parse\n'
            'print(urllib.parse.urlencode(os.environ))\n',
            'import os\n'
            'from urllib.parse import urlencode\n'
            'print(urlencode(os.environ))\n',
            'import os\n'
            'from urllib.parse import urlencode as encode\n'
            'query = encode(os.environ)\n'
            'print(query)\n',
            'import os\n'
            'import urllib.parse as parse\n'
            'secret = os.environ\n'
            'print(parse.urlencode(secret))\n',
            'import os\n'
            'from urllib.parse import urlencode as encode\n'
            'def unrelated(encode):\n'
            '    return "reviewed"\n'
            'secret = os.environ\n'
            'print(encode(secret))\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'from urllib.parse import urlencode\n'
            'print(urlencode({"status": "reviewed"}))\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_sensitive_environment_assignment_to_members_keeps_taint(self) -> None:
        unsafe = (
            'import os\n'
            'box = {}\n'
            'box.payload = os.environ\n'
            'print(box.payload)\n',
            'import os\n'
            'box = {}\n'
            'box["payload"] = os.environ\n'
            'print(box["payload"])\n',
            'import os\n'
            'box = {}\n'
            'box.payload = os.environ\n'
            'value = box.payload\n'
            'print(value)\n',
            'import os\n'
            'box = {}\n'
            'box.payload = os.environ\n'
            'first = box.payload\n'
            'second = first\n'
            'print(second)\n',
            'import os\n'
            'def save(obj, value):\n'
            '    obj.payload = value\n'
            'box = {}\n'
            'save(box, os.environ)\n'
            'print(box.payload)\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'box = {}\n'
            'box["payload"] = {"status": "reviewed"}\n'
            'print(box["payload"])\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_environment_joins_cover_str_descriptor_and_nested_next(self) -> None:
        unsafe = (
            'import os\n'
            'secret = os.environ\n'
            'print(str.join("", secret.values()))\n',
            'import os\n'
            'secret = os.environ\n'
            'print("".join(next(iter(secret.values()))))\n',
            'import os\n'
            'def unrelated(str):\n'
            '    return "reviewed"\n'
            'secret = os.environ\n'
            'print(str.join("", secret.values()))\n',
            'import os\n'
            'def unrelated(next):\n'
            '    return "reviewed"\n'
            'secret = os.environ\n'
            'print("".join(next(iter(secret.values()))))\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'print(str.join("", {"status": "reviewed"}.values()))\n',
            'print("".join(next(iter({"status": "reviewed"}.values()))))\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_sensitive_taint_crosses_inline_lambda_and_static_method_parameters(self) -> None:
        unsafe = (
            'import os\n'
            '(lambda payload: print(payload))(os.environ)\n',
            'import os\n'
            'class C:\n'
            '    @staticmethod\n'
            '    def emit(payload):\n'
            '        print(payload)\n'
            'C().emit(os.environ)\n',
            'import os\n'
            'sm = staticmethod\n'
            'class C:\n'
            '    @sm\n'
            '    def emit(payload):\n'
            '        print(payload)\n'
            'C().emit(os.environ)\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'emit = lambda payload: print(payload)\n'
            'emit({"status": "reviewed"})\n',
            'class C:\n'
            '    @staticmethod\n'
            '    def emit(payload):\n'
            '        print(payload)\n'
            'C().emit({"status": "reviewed"})\n',
            'sm = staticmethod\n'
            'class C:\n'
            '    @sm\n'
            '    def emit(payload):\n'
            '        print(payload)\n'
            'C().emit({"status": "reviewed"})\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_sensitive_taint_reaches_string_format_arguments(self) -> None:
        unsafe = (
            'import os\n'
            'secret = os.environ\n'
            'print("{}".format(secret))\n',
            'import os\n'
            'print("{}".format(os.environ))\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'secret = {"status": "reviewed"}\n'
            'print("{}".format(secret))\n',
            'print("{}".format({"status": "reviewed"}))\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_format_callable_aliases_preserve_sensitive_taint(self) -> None:
        unsafe = (
            'import os\n'
            'secret = os.environ\n'
            'fmt = format\n'
            'print(fmt(secret))\n',
            'import os\n'
            'secret = os.environ\n'
            'fmt = "{}".format\n'
            'print(fmt(secret))\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'fmt = format\n'
            'print(fmt("reviewed"))\n',
            'fmt = "{}".format\n'
            'print(fmt({"status": "reviewed"}))\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_user_defined_format_alias_returning_constant_is_safe(self) -> None:
        safe = (
            'import os\n'
            'def format(value):\n'
            '    return "reviewed"\n'
            'print(format(os.environ))\n',
            'import os\n'
            'def format(value):\n'
            '    return "reviewed"\n'
            'fmt = format\n'
            'print(fmt(os.environ))\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_factory_returned_bound_method_receives_sensitive_argument(self) -> None:
        unsafe = (
            'import os\n'
            'class C:\n'
            '    def emit(self, value):\n'
            '        print(value)\n'
            'def make_callback():\n'
            '    return C().emit\n'
            'callback = make_callback()\n'
            'callback(os.environ)\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'class C:\n'
            '    def emit(self, value):\n'
            '        print(value)\n'
            'def make_callback():\n'
            '    return C().emit\n'
            'callback = make_callback()\n'
            'callback({"status": "reviewed"})\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_container_and_conditional_class_aliases_preserve_return_taint(self) -> None:
        unsafe = (
            'import os\n'
            'class Snapshot:\n'
            '    def read(self):\n'
            '        return dict(os.environ)\n'
            'Alias = (Snapshot,)[0]\n'
            'print(Alias().read())\n',
            'import os\n'
            'class Snapshot:\n'
            '    def read(self):\n'
            '        return dict(os.environ)\n'
            'class Status:\n'
            '    def read(self):\n'
            '        return {"status": "reviewed"}\n'
            'Alias = Snapshot if flag else Status\n'
            'print(Alias().read())\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'class StatusSnapshot:\n'
            '    def read(self):\n'
            '        return {"status": "reviewed"}\n'
            'Alias = (StatusSnapshot,)[0]\n'
            'print(Alias().read())\n',
            'class StatusSnapshot:\n'
            '    def read(self):\n'
            '        return {"status": "reviewed"}\n'
            'Alias = StatusSnapshot if flag else StatusSnapshot\n'
            'print(Alias().read())\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_sensitive_mapping_return_survives_unrelated_nested_name_collision(self) -> None:
        unsafe = (
            'import os\n'
            'def relay(value):\n'
            '    return value\n'
            'def build_snapshot():\n'
            '    return relay(dict(os.environ))\n'
            'def unrelated_scope():\n'
            '    def relay(other):\n'
            '        return {"status": "reviewed"}\n'
            'print(build_snapshot())\n'
        )
        self.assertIsNotNone(self.inspect(unsafe))

    def test_safe_top_level_helper_ignores_unrelated_nested_name_collision(self) -> None:
        safe_collision = (
            'import os\n'
            'def snapshot():\n'
            '    return {"status": "reviewed"}\n'
            'def unrelated_scope():\n'
            '    def snapshot():\n'
            '        return os.environ\n'
            'print(snapshot())\n'
        )
        self.assertIsNone(self.inspect(safe_collision))

    def test_resolved_local_paths_from_helpers_and_globals_reach_output_sinks(self) -> None:
        unsafe = (
            'from pathlib import Path\n'
            'def worktree_root():\n'
            '    return Path.cwd().resolve()\n'
            'print(worktree_root())\n',
            'from pathlib import Path\n'
            'resolved_root = Path.cwd().resolve()\n'
            'def report_root():\n'
            '    print(resolved_root)\n'
            'report_root()\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'from pathlib import Path\n'
            'def worktree_root():\n'
            '    return Path.cwd().resolve()\n'
            'root = worktree_root()\n'
            'if not root.is_absolute():\n'
            '    raise SystemExit("invalid root")\n'
        )
        self.assertIsNone(self.inspect(safe))

    def test_resolved_local_paths_cross_helper_boundaries_to_output_sinks(self) -> None:
        unsafe = (
            'from pathlib import Path\n'
            'def worktree_root():\n'
            '    return Path.cwd().resolve()\n'
            'root_alias = worktree_root\n'
            'print(root_alias())\n',
            'from pathlib import Path\n'
            'def report(root):\n'
            '    print(root)\n'
            'report(Path.cwd().resolve())\n',
            'from pathlib import Path\n'
            'def worktree_roots():\n'
            '    yield Path.cwd().resolve()\n'
            'print(next(worktree_roots()))\n',
            'from pathlib import Path\n'
            'def outer():\n'
            '    root = Path.cwd().resolve()\n'
            '    def report():\n'
            '        print(root)\n'
            '    report()\n'
            'outer()\n',
            'from pathlib import Path\n'
            'def report(root=Path.cwd().resolve()):\n'
            '    print(root)\n'
            'report()\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe_default_validation = (
            'from pathlib import Path\n'
            'def validate_root(root=Path.cwd().resolve()):\n'
            '    if not root.is_absolute():\n'
            '        raise SystemExit("invalid root")\n'
            'validate_root()\n'
        )
        self.assertIsNone(self.inspect(safe_default_validation))

    def test_resolved_local_paths_cross_expanded_helper_arguments(self) -> None:
        unsafe = (
            'from pathlib import Path\n'
            'def report(*roots):\n    print(roots)\n'
            'report(Path.cwd().resolve())\n',
            'from pathlib import Path\n'
            'def report(**roots):\n    print(roots)\n'
            'report(root=Path.cwd().resolve())\n',
            'from pathlib import Path\n'
            'def report(root):\n    print(root)\n'
            'report(**{"root": Path.cwd().resolve()})\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

    def test_resolved_paths_cross_expanded_keyword_helpers_and_formatting(self) -> None:
        unsafe = (
            'from pathlib import Path\n'
            'def report(root):\n'
            '    print(root)\n'
            'report(**dict(root=Path.cwd().resolve()))\n',
            'from pathlib import Path\n'
            'raise RuntimeError("root={}".format(Path.cwd().resolve()))\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'from pathlib import Path\n'
            'def validate(root):\n'
            '    if not root.is_absolute():\n'
            '        raise SystemExit("invalid root")\n'
            'validate(**dict(root=Path.cwd().resolve()))\n'
        )
        self.assertIsNone(self.inspect(safe))

    def test_resolved_local_paths_in_raised_errors_are_rejected(self) -> None:
        unsafe = (
            'from pathlib import Path\nraise RuntimeError(str(Path.cwd().resolve()))\n',
            'from pathlib import Path\nraise SystemExit(f"root={Path.cwd().resolve()}")\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect('raise RuntimeError("reviewed status")\n'))

    def test_sensitive_variadic_and_default_helper_parameters_are_tainted(self) -> None:
        unsafe = (
            'import os\ndef report(*values):\n    print(values)\n'
            'report(dict(os.environ))\n',
            'import os\ndef report(**values):\n    print(values)\n'
            'report(**dict(os.environ))\n',
            'import os\ndef report(value=dict(os.environ)):\n'
            '    print(value)\nreport()\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect(
            'def report(*values, **options):\n    print(values, options)\n'
            'report("reviewed", status="safe")\n'
        ))

    def test_sensitive_mapping_expanded_into_kwargs_is_tainted(self) -> None:
        unsafe = (
            'import os\n'
            'def report(**values):\n'
            '    print(values["snapshot"])\n'
            'report(**dict(snapshot=os.environ))\n',
            'import os\n'
            'def report(**values):\n'
            '    print(values)\n'
            'report(**dict(snapshot=dict(os.environ)))\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'def report(**values):\n'
            '    print(values)\n'
            'report(**dict(status="reviewed"))\n'
        )
        self.assertIsNone(self.inspect(safe))

    def test_sys_exit_is_an_output_sink_for_sensitive_values(self) -> None:
        unsafe = (
            'import os, sys\nsys.exit(str(dict(os.environ)))\n',
            'import os, sys\ndef snapshot():\n    return dict(os.environ)\n'
            'sys.exit(str(snapshot()))\n',
            'from pathlib import Path\nimport sys\n'
            'sys.exit(str(Path.cwd().resolve()))\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect('import sys\nsys.exit("reviewed status")\n'))

    def test_imported_exit_alias_and_system_exit_preserve_sensitive_taint(self) -> None:
        unsafe = (
            'from sys import exit as leave\n'
            'import os\n'
            'leave(str(dict(os.environ)))\n',
            'import os\n'
            'raise SystemExit(str(dict(os.environ)))\n',
            'import os, sys\n'
            'leave = sys.exit\n'
            'leave(str(dict(os.environ)))\n',
            'import os\n'
            'abort = SystemExit\n'
            'raise abort(dict(os.environ))\n',
            'import os, sys\n'
            'leave = [sys.exit][0]\n'
            'leave(str(dict(os.environ)))\n',
            'import os\n'
            'abort = [SystemExit][0]\n'
            'raise abort(dict(os.environ))\n',
            'import os, sys\n'
            'leave = getattr(sys, "exit")\n'
            'leave(str(dict(os.environ)))\n',
            'import os, sys\n'
            'leave = getattr(sys, "exit", None)\n'
            'leave(str(dict(os.environ)))\n',
            'import os, sys\n'
            'member = "exit"\n'
            'leave = getattr(sys, member, None)\n'
            'leave(str(dict(os.environ)))\n',
            'import os, sys\n'
            'leave = getattr(object(), "missing", sys.exit)\n'
            'leave(str(dict(os.environ)))\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'from sys import exit as leave\n'
            'leave("reviewed status")\n'
            'raise SystemExit("reviewed status")\n'
        )
        self.assertIsNone(self.inspect(safe))

    def test_canonical_package_guard_remains_reviewed(self) -> None:
        bodies = [
            body
            for _line, body, _safe_marker, _invocation
            in self.scanner["python_heredoc_bodies"](PACKET_TEXT)  # type: ignore[operator]
            if "def source_fuzz_guard():" in body
        ]
        self.assertEqual(len(bodies), 1)
        self.assertIsNone(self.inspect(bodies[0]))
        mutated_root = bodies[0].replace(
            "invocation_root = Path.cwd().resolve()",
            'invocation_root = Path("/synthetic/unreviewed-root").resolve()',
            1,
        )
        self.assertNotEqual(mutated_root, bodies[0])
        self.assertIsNotNone(self.inspect(mutated_root))
        shadowed_root = bodies[0].replace(
            "def source_fuzz_guard():\n    package_dir =",
            'def source_fuzz_guard():\n'
            '    go_repo_root = Path("/synthetic/unreviewed-root")\n'
            '    package_dir =',
            1,
        )
        self.assertNotEqual(shadowed_root, bodies[0])
        self.assertIsNotNone(self.inspect(shadowed_root))

    def test_environment_taint_reaches_loop_and_comprehension_targets(self) -> None:
        loop = 'import os\nfor value in os.environ.values():\n    print(value)\n'
        comprehension = 'import os\n[print(value) for value in os.environ.values()]\n'
        wrapped = (
            'import os\nfor _, value in enumerate(os.environ.values()):\n'
            '    print(value)\n'
        )
        zipped = (
            'import os\nfor _, value in zip(range(1), os.environ.values()):\n'
            '    print(value)\n'
        )
        starred = (
            'import os\nfor *secret, in os.environ.values():\n'
            '    print(secret)\n'
        )
        for body in (loop, comprehension, wrapped, zipped, starred):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        safe = 'for value in ["reviewed"]:\n    print(value)\n'
        self.assertIsNone(self.inspect(safe))

    def test_environment_taint_reaches_comprehension_iterator_outputs(self) -> None:
        unsafe = (
            'import os\n'
            'print([value for value in iter(dict(os.environ).items())])\n',
            'import os\n'
            'values = [value for _, value in enumerate(dict(os.environ).items())]\n'
            'print(values)\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = 'print([value for value in ["reviewed"]])\n'
        self.assertIsNone(self.inspect(safe))

    def test_environment_taint_follows_generator_yields(self) -> None:
        body = (
            'import os\n'
            'def inherited_values():\n'
            '    yield from os.environ.values()\n'
            'for secret in inherited_values():\n'
            '    print(secret)\n'
        )
        self.assertIsNotNone(self.inspect(body))

    def test_environment_taint_follows_itertools_chain(self) -> None:
        body = (
            'import itertools\nimport os\n'
            'for secret in itertools.chain(("reviewed",), os.environ.values()):\n'
            '    print(secret)\n'
        )
        tree = ast.parse(body, filename="<environment-taint-data>")
        parents = {
            child: parent
            for parent in ast.walk(tree)
            for child in ast.iter_child_nodes(parent)
        }
        tainted_names = self.scanner["python_sensitive_value_names"](tree, parents)  # type: ignore[operator]
        self.assertIn("secret", tainted_names)

    def test_environment_taint_follows_starred_operands(self) -> None:
        body = (
            'import os\n'
            'for secret in zip(*[os.environ.values()]):\n'
            '    print(secret)\n'
        )
        self.assertIsNotNone(self.inspect(body))

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
        mapping_view = (
            'import subprocess\n'
            'launchers = {"run": subprocess.run}\n'
            'for launch in launchers.values():\n'
            '    launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        literal_mapping_view = (
            'import subprocess\n'
            'for launch in {"run": subprocess.run}.values():\n'
            '    launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        converted_container = (
            'import subprocess\n'
            'launchers = {"run": subprocess.run}\n'
            'for launch in list(launchers.values()):\n'
            '    launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        starred_subscript = (
            'import subprocess\n'
            'for *launchers, in [subprocess.run]:\n'
            '    launchers[0](["gh", "workflow", "run", "ci.yml"])\n'
        )
        for body in (
            direct, container, comprehension, mapping_view,
            literal_mapping_view, converted_container, starred_subscript,
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        safe = 'for transform in [str.upper]:\n    transform("reviewed")\n'
        self.assertIsNone(self.inspect(safe))

    def test_launcher_aliases_survive_reversed_and_dict_conversions(self) -> None:
        reversed_values = (
            'import subprocess\n'
            'launchers = [subprocess.run]\n'
            'for launch in reversed(launchers):\n'
            '    launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        dictionary_values = (
            'import subprocess\n'
            'launchers = {"run": subprocess.run}\n'
            'for launch in dict(launchers).values():\n'
            '    launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        for body in (reversed_values, dictionary_values):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

    def test_launcher_aliases_survive_sorted_mapping_values(self) -> None:
        body = (
            'import subprocess\n'
            'launchers = {"run": subprocess.run}\n'
            'for launch in sorted(launchers.values()):\n'
            '    launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        self.assertIsNotNone(self.inspect(body))

    def test_launcher_aliases_survive_filter_mapping_values(self) -> None:
        body = (
            'import subprocess\n'
            'launchers = {"run": subprocess.run}\n'
            'for launch in filter(None, launchers.values()):\n'
            '    launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        self.assertIsNotNone(self.inspect(body))

    def test_launcher_aliases_survive_map_mapping_values(self) -> None:
        body = (
            'import subprocess\n'
            'launchers = {"run": subprocess.run}\n'
            'for launch in map(lambda value: value, launchers.values()):\n'
            '    launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        self.assertIsNotNone(self.inspect(body))

    def test_launcher_aliases_yielded_by_local_generator(self) -> None:
        body = (
            'import subprocess\n'
            'def launcher_stream():\n'
            '    yield subprocess.run\n'
            'for launch in launcher_stream():\n'
            '    launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        self.assertIsNotNone(self.inspect(body))

    def test_launcher_aliases_returned_by_local_helpers_are_rejected(self) -> None:
        body = (
            'import subprocess\n'
            'def launcher_factory():\n'
            '    return subprocess.run\n'
            'launch = launcher_factory()\n'
            'launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        self.assertIsNotNone(self.inspect(body))

    def test_launcher_aliases_returned_by_methods_and_lambdas_are_rejected(self) -> None:
        unsafe = (
            'import subprocess\n'
            'launcher_factory = lambda: subprocess.run\n'
            'launcher = launcher_factory()\n'
            'launcher(["gh", "workflow", "run", "ci.yml"])\n',
            'import subprocess\n'
            'launcher = (lambda: subprocess.run)()\n'
            'launcher(["gh", "workflow", "run", "ci.yml"])\n',
            'import subprocess\n'
            'class LauncherFactory:\n'
            '    def get(self):\n'
            '        return subprocess.run\n'
            'launcher = LauncherFactory().get()\n'
            'launcher(["gh", "workflow", "run", "ci.yml"])\n',
            'import subprocess\n'
            'class LauncherFactory:\n'
            '    def __init__(self, label):\n'
            '        self.label = label\n'
            '    def get(self):\n'
            '        return subprocess.run\n'
            'factory = LauncherFactory("reviewed")\n'
            'alias = factory\n'
            'launcher = alias.get()\n'
            'launcher(["gh", "workflow", "run", "ci.yml"])\n',
            'import subprocess\n'
            'class LauncherFactory:\n'
            '    def get(self):\n'
            '        return subprocess.run\n'
            'factory = LauncherFactory()\n'
            'get_launcher = factory.get\n'
            'launcher = get_launcher()\n'
            'launcher(["gh", "workflow", "run", "ci.yml"])\n',
            'import subprocess\n'
            'def make_launcher():\n'
            '    return subprocess.run\n'
            'class LauncherFactory:\n'
            '    pass\n'
            'factory = LauncherFactory()\n'
            'get_launcher = getattr(factory, "get", make_launcher)\n'
            'launcher = get_launcher()\n'
            'launcher(["gh", "workflow", "run", "ci.yml"])\n',
            'import subprocess\n'
            'class LauncherFactory:\n'
            '    def get(self):\n'
            '        return subprocess.run\n'
            'factory = LauncherFactory()\n'
            'get_launcher = getattr(factory, "get")\n'
            'launcher = get_launcher()\n'
            'launcher(["gh", "workflow", "run", "ci.yml"])\n',
            'import subprocess\n'
            'class LauncherFactory:\n'
            '    def get(self):\n'
            '        return subprocess.run\n'
            'factory = LauncherFactory()\n'
            'get_launcher = getattr(factory, "get", None)\n'
            'launcher = get_launcher()\n'
            'launcher(["gh", "workflow", "run", "ci.yml"])\n',
            'import subprocess\n'
            'class LauncherFactory:\n'
            '    def get(self):\n'
            '        return subprocess.run\n'
            'factory = LauncherFactory()\n'
            'member = "get"\n'
            'get_launcher = getattr(factory, member, None)\n'
            'launcher = get_launcher()\n'
            'launcher(["gh", "workflow", "run", "ci.yml"])\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'class StatusFactory:\n'
            '    def get(self):\n'
            '        return str.upper\n'
            'transform = StatusFactory().get()\n'
            'transform("reviewed")\n'
        )
        self.assertIsNone(self.inspect(safe))

    def test_launcher_alias_returned_by_mapping_pop_is_rejected(self) -> None:
        unsafe = (
            'import subprocess\n'
            'launchers = {"x": subprocess.run}\n'
            'launch = launchers.pop("x")\n'
            'launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        self.assertIsNotNone(self.inspect(unsafe))

        safe = (
            'callbacks = {"upper": str.upper}\n'
            'transform = callbacks.pop("upper")\n'
            'transform("reviewed")\n'
        )
        self.assertIsNone(self.inspect(safe))

    def test_launcher_alias_returned_by_mapping_get_is_rejected(self) -> None:
        unsafe = (
            'import subprocess\n'
            'launchers = {"x": subprocess.run}\n'
            'launch = launchers.get("x")\n'
            'launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        self.assertIsNotNone(self.inspect(unsafe))

        safe = (
            'callbacks = {"upper": str.upper}\n'
            'transform = callbacks.get("upper")\n'
            'transform("reviewed")\n'
        )
        self.assertIsNone(self.inspect(safe))

    def test_mapping_lookup_method_alias_chain_preserves_launcher_provenance(self) -> None:
        unsafe = (
            'import subprocess\n'
            'launchers = {"x": subprocess.run}\n'
            'lookup = launchers.get\n'
            'lookup2 = lookup\n'
            'launch = lookup2("x")\n'
            'launch(["gh", "workflow", "run", "ci.yml"])\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'callbacks = {"upper": str.upper}\n'
            'lookup = callbacks.get\n'
            'lookup2 = lookup\n'
            'transform = lookup2("upper")\n'
            'transform("reviewed")\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_isolated_invocation_ignores_synthetic_local_module(self) -> None:
        with tempfile.TemporaryDirectory(prefix="issue79-python-isolation-") as root:
            synthetic_module = Path(root) / "json.py"
            synthetic_module.write_text('VALUE = "synthetic"\n', encoding="utf-8")
            result = subprocess.run(
                [
                    sys.executable,
                    "-I",
                    "-B",
                    "-c",
                    'import json; print(getattr(json, "VALUE", "stdlib"))',
                ],
                cwd=root,
                env={"PYTHONPATH": root},
                stdin=subprocess.DEVNULL,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                check=False,
            )
            self.assertEqual(0, result.returncode, "isolated interpreter probe failed")
            self.assertEqual("stdlib", result.stdout.strip())

        adr = (ROOT / "docs" / "decisions" / "0004-offline-python-ast-regression-tooling.md").read_text(
            encoding="utf-8"
        )
        self.assertIn("python3 -I -B", adr)
        self.assertNotIn("Invoke it with `python3 -B`", adr)

    def test_bounded_git_query_loader_rejects_unreviewed_function_definitions(self) -> None:
        specimen = ast.parse(
            'def run_bounded_git_query(value=packet_side_effect()):\n'
            '    return value\n',
            filename="<function-definition-data>",
        ).body[0]
        self.assertIsInstance(specimen, ast.FunctionDef)
        compile_events: list[str] = []
        exec_events: list[str] = []
        original_compile = builtins.compile
        original_exec = builtins.exec

        def record_compile(*_args: object, **_kwargs: object) -> None:
            compile_events.append("compile")

        def record_exec(*_args: object, **_kwargs: object) -> None:
            exec_events.append("exec")

        builtins.compile = record_compile  # type: ignore[assignment]
        builtins.exec = record_exec  # type: ignore[assignment]
        try:
            with self.assertRaises(AssertionError):
                _bounded_git_query_namespace(
                    ast.Module(body=[specimen], type_ignores=[])
                )
        finally:
            builtins.compile = original_compile
            builtins.exec = original_exec
        self.assertEqual([], compile_events)
        self.assertEqual([], exec_events)

    def test_parity_helper_rejects_unreviewed_definition_before_compile(self) -> None:
        original_verification = self.verification
        specimen = ast.parse(
            '@packet_side_effect()\n'
            'def require_packet_head_parity(intent, blob, worktree):\n'
            '    return None\n',
            filename="<function-definition-data>",
        ).body[0]
        self.assertIsInstance(specimen, ast.FunctionDef)
        self.verification = ast.Module(body=[specimen], type_ignores=[])
        compile_events: list[str] = []
        exec_events: list[str] = []
        original_compile = builtins.compile
        original_exec = builtins.exec

        def record_compile(*_args: object, **_kwargs: object) -> None:
            compile_events.append("compile")

        def record_exec(*_args: object, **_kwargs: object) -> None:
            exec_events.append("exec")

        builtins.compile = record_compile  # type: ignore[assignment]
        builtins.exec = record_exec  # type: ignore[assignment]
        try:
            with self.assertRaises(AssertionError):
                self.test_final_parity_helper_rejects_intent_bits_and_raw_byte_divergence()
        finally:
            builtins.compile = original_compile
            builtins.exec = original_exec
            self.verification = original_verification
        self.assertEqual([], compile_events)
        self.assertEqual([], exec_events)

    def test_verification_parity_helper_definition_must_be_unique(self) -> None:
        duplicate = ast.parse(
            'def require_packet_head_parity(intent, blob, worktree):\n'
            '    return None\n'
            'def require_packet_head_parity(intent, blob, worktree):\n'
            '    return None\n',
            filename="<duplicate-parity-helper-data>",
        )
        with self.assertRaises(AssertionError):
            _verification_function(duplicate, "require_packet_head_parity")

    def test_environment_dump_builtins_are_narrowly_allowed(self) -> None:
        for command in (
            "export", "export -p", "set", "set -o posix", "env", "env -0",
            "env -u NAME", "env NAME=synthetic",
        ):
            with self.subTest(command=command):
                self.assertIsNotNone(self.shell_violation(command))
        for command in (
            "set -euo pipefail",
            "export PATH=/opt/homebrew/bin:/usr/bin:/bin",
            "export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null",
            "export GIT_CONFIG_COUNT=2 GIT_CONFIG_KEY_0=core.fsmonitor GIT_CONFIG_VALUE_0=false GIT_CONFIG_KEY_1=core.hooksPath GIT_CONFIG_VALUE_1=/dev/null",
            "env -i printf reviewed",
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
        self.assertEqual(child_environment.get("GIT_CONFIG_SYSTEM"), "/dev/null")
        query_runtime = _bounded_git_query_namespace(self.verification)
        query = query_runtime["git_query"](["status", "--short"])
        self.assertEqual(
            query,
            [
                "/usr/bin/env", "-i", "GIT_CONFIG_NOSYSTEM=1",
                "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
                "GIT_ATTR_NOSYSTEM=1", "/usr/bin/git", "--no-replace-objects",
                "-P", "-c", "core.fsmonitor=false", "-c",
                "core.hooksPath=/dev/null", "status", "--short",
            ],
        )

    def test_final_parity_helper_rejects_intent_bits_and_raw_byte_divergence(self) -> None:
        function = _verification_function(self.verification, "require_packet_head_parity")
        verifier_text = PACKET_TEXT[
            PACKET_TEXT.index("The following dynamic command is the live final-verification template."):
        ]
        reviewed_paths = ast.literal_eval(
            _top_level_assignment(
                self.verification, "issue79_reviewed_evidence_paths"
            ).value
        )
        self.assertEqual(
            reviewed_paths,
            (
                "docs/evidence/g01-recovery-packet.md",
                "scripts/evidence_packet/issue79_regression_test.py",
                "docs/decisions/0004-offline-python-ast-regression-tooling.md",
            ),
        )
        required_order = (
            'git_query(["rev-parse", "HEAD"])',
            'for reviewed_path in issue79_reviewed_evidence_paths:',
            'git_query(["ls-files", "-v", "-z", "--", reviewed_path])',
            'run_bounded_git_packet_blob_query(\n        f"{local}:{reviewed_path}"',
            'Path(\n                "docs/evidence/g01-recovery-packet.md"\n            ).read_bytes()',
            'Path(\n                "scripts/evidence_packet/issue79_regression_test.py"\n            ).read_bytes()',
            'Path(\n                "docs/decisions/0004-offline-python-ast-regression-tooling.md"\n            ).read_bytes()',
            "require_packet_head_parity(\n        intent_result.stdout",
            'git_query(["status", "--porcelain=v1", "--untracked-files=all"])',
        )
        order = [verifier_text.index(item) for item in required_order]
        self.assertEqual(order, sorted(order))
        namespace: dict[str, object] = {
            "__builtins__": __builtins__,
        }
        _validate_packet_function_definition(
            function, "require_packet_head_parity"
        )
        exec(
            compile(ast.Module(body=[function], type_ignores=[]), "<parity-helper>", "exec"),
            namespace,
        )
        helper = namespace["require_packet_head_parity"]
        with tempfile.TemporaryDirectory(prefix="gh-runnerd-issue79-") as directory:
            root = Path(directory)
            relative_paths = tuple(Path(path) for path in reviewed_paths)
            reviewed_bytes = {
                relative: f"synthetic reviewed bytes for {relative.as_posix()}\x00\n".encode()
                for relative in relative_paths
            }
            changed_bytes = {
                relative: f"synthetic modified bytes for {relative.as_posix()}\x00\n".encode()
                for relative in relative_paths
            }
            for relative in relative_paths:
                tracked_path = root / relative
                tracked_path.parent.mkdir(parents=True, exist_ok=True)
                tracked_path.write_bytes(reviewed_bytes[relative])
            env = {
                "PATH": "/usr/bin:/bin",
                "HOME": directory,
                "GIT_CONFIG_NOSYSTEM": "1",
                "GIT_CONFIG_GLOBAL": os.devnull,
                "GIT_CONFIG_SYSTEM": os.devnull,
                "LC_ALL": "C",
            }
            _run_git_checked(["init", "-q"], root, env)
            _run_git_checked(
                ["add", *(relative.as_posix() for relative in relative_paths)],
                root,
                env,
            )
            _run_git_checked(
                ["-c", "user.name=synthetic", "-c", "user.email=synthetic@example.invalid", "commit", "-q", "-m", "baseline"],
                root,
                env,
            )
            head = _run_git_checked(
                ["rev-parse", "HEAD"], root, env
            ).decode().strip()
            for relative in relative_paths:
                intent = _run_git_checked(
                    ["ls-files", "-v", "-z", "--", relative.as_posix()], root, env
                )
                blob = _run_git_checked(
                    ["show", f"{head}:{relative.as_posix()}"], root, env
                )
                helper(intent, blob, (root / relative).read_bytes())

            for flag, clear_flag in (
                ("--skip-worktree", "--no-skip-worktree"),
                ("--assume-unchanged", "--no-assume-unchanged"),
            ):
                for relative in relative_paths:
                    tracked_path = root / relative
                    _run_git_checked(
                        ["update-index", flag, relative.as_posix()], root, env
                    )
                    tracked_path.write_bytes(changed_bytes[relative])
                    legacy_status = _run_git_checked(
                        ["status", "--porcelain=v1", "--untracked-files=all"], root, env
                    )
                    self.assertEqual(legacy_status, b"")
                    with self.assertRaises(SystemExit):
                        helper(
                            _run_git_checked(
                                ["ls-files", "-v", "-z", "--", relative.as_posix()],
                                root,
                                env,
                            ),
                            _run_git_checked(
                                ["show", f"{head}:{relative.as_posix()}"], root, env
                            ),
                            tracked_path.read_bytes(),
                        )
                    _run_git_checked(
                        ["update-index", clear_flag, relative.as_posix()], root, env
                    )
                    tracked_path.write_bytes(reviewed_bytes[relative])

            for relative in relative_paths:
                tracked_path = root / relative
                tracked_path.write_bytes(changed_bytes[relative])
                with self.assertRaises(SystemExit):
                    helper(
                        _run_git_checked(
                            ["ls-files", "-v", "-z", "--", relative.as_posix()],
                            root,
                            env,
                        ),
                        _run_git_checked(
                            ["show", f"{head}:{relative.as_posix()}"], root, env
                        ),
                        tracked_path.read_bytes(),
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

    def test_packet_blob_query_ignores_replace_refs(self) -> None:
        runtime = _bounded_git_query_namespace(self.verification)
        self.assertIn(
            "--no-replace-objects",
            runtime["git_query"](["rev-parse", "HEAD"]),  # type: ignore[operator]
        )
        with tempfile.TemporaryDirectory(prefix="gh-runnerd-issue79-replace-") as directory:
            root = Path(directory)
            relative = Path("docs/evidence/g01-recovery-packet.md")
            packet = root / relative
            packet.parent.mkdir(parents=True)
            reviewed_bytes = b"synthetic reviewed HEAD packet\n"
            replacement_bytes = b"synthetic replacement packet\n"
            packet.write_bytes(reviewed_bytes)
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
                    "commit", "-q", "-m", "reviewed",
                ],
                root,
                env,
            )
            reviewed_head = _run_git_checked(["rev-parse", "HEAD"], root, env).decode().strip()
            packet.write_bytes(replacement_bytes)
            _run_git_checked(["add", relative.as_posix()], root, env)
            _run_git_checked(
                [
                    "-c", "user.name=synthetic",
                    "-c", "user.email=synthetic@example.invalid",
                    "commit", "--amend", "-q", "--no-edit",
                ],
                root,
                env,
            )
            replacement_head = _run_git_checked(["rev-parse", "HEAD"], root, env).decode().strip()
            _run_git_checked(["switch", "--detach", reviewed_head], root, env)
            _run_git_checked(["replace", reviewed_head, replacement_head], root, env)

            replaced_blob = _run_git_checked(
                ["show", f"{reviewed_head}:{relative.as_posix()}"], root, env
            )
            self.assertEqual(replaced_blob, replacement_bytes)
            result = runtime["run_bounded_git_packet_blob_query"](
                f"{reviewed_head}:{relative.as_posix()}", cwd=root, env=env
            )
            self.assertEqual(result.stdout, reviewed_bytes)

    def test_git_config_include_options_are_rejected_before_read_only_classification(self) -> None:
        for command in (
            "git -c include.path=synthetic/included.cfg status",
            "git -c includeIf.gitdir:/synthetic/repo.path=synthetic/included.cfg status",
            "git -cinclude.path=synthetic/included.cfg status",
            "git --config-env=include.path=SYNTHETIC_INCLUDE status",
            "git config --includes --list",
            "git config --incl --list",
            "git config --inc --list",
            "GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=include.path GIT_CONFIG_VALUE_0=synthetic/included.cfg git status",
            "env GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=includeIf.gitdir:/synthetic/repo.path GIT_CONFIG_VALUE_0=synthetic/included.cfg git status",
            "GIT_CONFIG_PARAMETERS=\"'include.path=synthetic/included.cfg'\" git status",
            "env GIT_CONFIG_PARAMETERS=\"'includeIf.gitdir:/synthetic/repo.path=synthetic/included.cfg'\" git status",
        ):
            with self.subTest(command=command):
                self.assertIsNotNone(self.shell_violation(command))
        for command in (
            "git -P status",
            "git -c core.fsmonitor=false status",
            _isolated_git_shell_command(["status"]).replace(
                "-c core.hooksPath=/dev/null", "-c core.hooksPath=/tmp/synthetic-hook"
            ),
        ):
            with self.subTest(command=command):
                self.assertIsNotNone(self.shell_violation(command))

    def test_core_worktree_override_cannot_mask_a_dirty_invocation_worktree(self) -> None:
        with tempfile.TemporaryDirectory(prefix="issue79-core-worktree-") as temporary:
            root = Path(temporary)
            repository = root / "invocation"
            alternate = root / "alternate"
            repository.mkdir()
            alternate.mkdir()
            (root / "home").mkdir()
            environment = {
                "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
                "HOME": str(root / "home"),
                "LC_ALL": "C",
                "GIT_CONFIG_NOSYSTEM": "1",
                "GIT_CONFIG_GLOBAL": "/dev/null",
                "GIT_CONFIG_SYSTEM": "/dev/null",
                "GIT_TERMINAL_PROMPT": "0",
            }
            _run_git_checked(["init", "-q"], repository, environment)
            tracked = repository / "tracked.txt"
            tracked.write_text("reviewed\n", encoding="utf-8")
            _run_git_checked(["add", "tracked.txt"], repository, environment)
            _run_git_checked(
                [
                    "-c", "user.name=synthetic",
                    "-c", "user.email=synthetic@example.invalid",
                    "commit", "-q", "-m", "fixture",
                ],
                repository,
                environment,
            )
            root_guard = _verification_function(
                self.verification, "require_git_invocation_root"
            )
            _validate_packet_function_definition(
                root_guard, "require_git_invocation_root"
            )
            runtime = _bounded_git_query_namespace(self.verification)
            exec(
                compile(
                    ast.Module(body=[root_guard], type_ignores=[]),
                    "<synthetic-git-root-guard>",
                    "exec",
                ),
                runtime,
            )
            runtime["require_git_invocation_root"](repository, environment)
            (alternate / "tracked.txt").write_text("reviewed\n", encoding="utf-8")
            tracked.write_text("dirty invocation worktree\n", encoding="utf-8")
            _run_git_checked(
                ["config", "--local", "core.worktree", str(alternate)],
                repository,
                environment,
            )
            redirected_status = _run_git_checked(
                ["status", "--short"], repository, environment
            )
            self.assertEqual(b"", redirected_status)
            self.assertEqual(b"dirty invocation worktree\n", tracked.read_bytes())
            with self.assertRaisesRegex(
                SystemExit,
                "post-correction Git top-level does not match invocation root",
            ):
                runtime["require_git_invocation_root"](repository, environment)

        # This command remains inert scanner data; only the disposable fixture
        # above is used to demonstrate the worktree-selection failure mode.
        self.assertIsNotNone(
            self.shell_violation(
                "git -c core.worktree=/synthetic/alternate status --short"
            )
        )
        self.assertIsNone(self.shell_violation(_isolated_git_shell_command(["status", "--short"])))

    def test_unbounded_git_config_dumps_are_rejected(self) -> None:
        for command in (
            "git config --global --list --show-origin",
            "git config --get-regexp .",
        ):
            with self.subTest(command=command):
                self.assertIsNotNone(self.shell_violation(command))
        for command in (
            "git config --get core.repositoryformatversion",
            "git config --local --get-regexp '^filter\\.'",
        ):
            with self.subTest(command=command):
                self.assertIsNone(
                    self.shell_violation(
                        _isolated_git_shell_command(shlex.split(command)[1:])
                    )
                )
        self.assertIsNone(
            self.shell_violation(_isolated_git_shell_command(["status", "--short"]))
        )

    def test_git_config_queries_allow_only_reviewed_keys(self) -> None:
        for command in (
            "git config --get credential.helper",
            "git config --get-all credential.helper",
            "git config --get-urlmatch http.extraheader https://github.com/1XP-AI/gh-runnerd",
            "git config --get-urlmatch http.https://github.com/.extraheader https://github.com/1XP-AI/gh-runnerd",
            "git config --get user.email",
            "git config --get-all user.email",
        ):
            with self.subTest(command=command):
                self.assertIsNotNone(self.shell_violation(command))

        for command in (
            "git config --get core.repositoryformatversion",
            "git config --local --get-regexp '^filter\\.'",
            "git config --get-urlmatch http.sslverify https://github.com/1XP-AI/gh-runnerd",
        ):
            with self.subTest(command=command):
                self.assertIsNone(
                    self.shell_violation(_isolated_git_shell_command(shlex.split(command)[1:]))
                )

    def test_shell_origin_url_query_is_rejected_but_verifier_capture_remains(self) -> None:
        query = "git config --local --get-all remote.origin.url"
        safe_query = _isolated_git_shell_command(shlex.split(query)[1:])
        safe_tokens = self.scanner["executable_tokens"](shlex.split(safe_query))  # type: ignore[operator]
        self.assertIsNone(
            self.scanner["git_read_only_violation"](safe_tokens)  # type: ignore[operator]
        )
        self.assertIsNotNone(self.shell_violation(safe_query))
        self.assertIsNotNone(
            self.shell_violation("trap 'git config --local --get-all remote.origin.url' EXIT")
        )
        self.assertIsNotNone(
            self.inspect(
                'import subprocess\n'
                'subprocess.run(["git", "config", "--local", "--get-all", '
                '"remote.origin.url"], check=True)\n'
            )
        )
        canonical_builder_prefix = (
            'import subprocess\n'
            'git_environment = {"PATH": "/usr/bin:/bin", '
            '"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", '
            '"GIT_CONFIG_SYSTEM": "/dev/null", "GIT_ATTR_NOSYSTEM": "1"}\n'
            'def git_command(arguments):\n'
            '    return ["/usr/bin/env", "-i", "GIT_CONFIG_NOSYSTEM=1", '
            '"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", '
            '"GIT_ATTR_NOSYSTEM=1", "/usr/bin/git", "--no-replace-objects", '
            '"-P", "-c", "core.fsmonitor=false", "-c", '
            '"core.hooksPath=/dev/null", *arguments]\n'
        )
        unsafe_builders = (
            'subprocess.run(git_command(["config", "--local", "--get-all", '
            '"remote.origin.url"]), env=git_environment)\n',
            'argv = git_command(["config", "--local", "--get-all", '
            '"remote.origin.url"])\n'
            'subprocess.run(argv, env=git_environment)\n',
            'subprocess.run(git_command(["config", "--local", "--get-all", '
            '"remote.origin.url"]), env=git_environment, '
            'stdout=subprocess.PIPE, stderr=subprocess.PIPE)\n',
        )
        for body in unsafe_builders:
            with self.subTest(body=body):
                self.assertIsNotNone(
                    self.inspect(canonical_builder_prefix + body),
                    "an isolated builder must not expose raw origin URL output",
                )
        self.assertIsNone(
            self.inspect(
                canonical_builder_prefix
                + 'subprocess.run(git_command(["status", "--short"]), '
                'env=git_environment)\n'
            ),
            "a non-sensitive isolated status query remains accepted",
        )

        origin_capture = _top_level_assignment(self.verification, "origin_result")
        self.assertEqual(
            self.scanner["python_dotted_name"](origin_capture.value.func),  # type: ignore[operator,union-attr]
            "run_bounded_git_query",
        )
        self.assertIn("remote.origin.url", ast.unparse(origin_capture.value))
        expected_origin = _top_level_assignment(self.verification, "expected_origin_url")
        self.assertEqual(
            _literal_assignment_value(expected_origin),
            "https://github.com/1XP-AI/gh-runnerd.git",
        )
        origin_check = next(
            statement
            for statement in self.verification.body
            if isinstance(statement, ast.If)
            and any(
                isinstance(node, ast.Name) and node.id == "origin_urls"
                for node in ast.walk(statement.test)
            )
        )
        self.assertIsInstance(origin_check.test, ast.Compare)

        bounded_helper = _verification_function(self.verification, "run_bounded_git_query")
        popen = next(
            node
            for node in ast.walk(bounded_helper)
            if isinstance(node, ast.Call)
            and self.scanner["python_dotted_name"](node.func) == "subprocess.Popen"  # type: ignore[operator]
        )
        popen_keywords = {keyword.arg: keyword.value for keyword in popen.keywords}
        for stream in ("stdout", "stderr"):
            self.assertEqual(
                self.scanner["python_dotted_name"](popen_keywords[stream]),  # type: ignore[operator]
                "subprocess.PIPE",
            )

        bounded_capture = _verification_function(self.verification, "capture_git_query_output")
        budget_guards = [
            node
            for node in ast.walk(bounded_capture)
            if isinstance(node, ast.Compare)
            and len(node.ops) == 1
            and isinstance(node.ops[0], ast.Gt)
            and len(node.comparators) == 1
            and isinstance(node.comparators[0], ast.Name)
            and node.comparators[0].id == "output_limit"
            and isinstance(node.left, ast.BinOp)
            and isinstance(node.left.op, ast.Add)
        ]
        self.assertEqual(1, len(budget_guards))
        bounded_lengths = [
            node
            for node in ast.walk(budget_guards[0].left)
            if isinstance(node, ast.Call)
            and self.scanner["python_dotted_name"](node.func) == "len"  # type: ignore[operator]
            and len(node.args) == 1
        ]
        captured_streams = {
            node.args[0].slice.value
            for node in bounded_lengths
            if isinstance(node.args[0], ast.Subscript)
            and isinstance(node.args[0].value, ast.Name)
            and node.args[0].value.id == "captures"
            and isinstance(node.args[0].slice, ast.Constant)
        }
        self.assertEqual({"stdout", "stderr"}, captured_streams)
        self.assertTrue(
            any(
                isinstance(node.args[0], ast.Name)
                and node.args[0].id == "chunk"
                for node in bounded_lengths
            )
        )

    def test_origin_capture_checker_remains_self_audit_compatible(self) -> None:
        scanner_body, marker = self.packet_heredoc_containing(
            "def reviewed_python_git_origin_capture_is_compared(tree, parents):",
            "isinstance(value.comparators[0].value, int)",
            "not isinstance(value.comparators[0].value, bool)",
            "def python_git_origin_capture_violation(tree, parents):",
        )
        self.assertIsNone(
            self.scanner["inspect_python_heredoc"](scanner_body, marker),  # type: ignore[operator]
            "the current origin-capture proof must pass the scanner's own source checks",
        )

    def test_git_query_capture_must_be_on_the_active_result_path(self) -> None:
        bounded_body, marker = self.packet_heredoc_containing(
            "def run_bounded_git_query(",
            "stdout, stderr = capture_git_query_output(",
        )
        self.assertIsNone(self.scanner["inspect_python_heredoc"](bounded_body, marker))  # type: ignore[operator]
        tree = ast.parse(bounded_body, filename="<bounded-query-capture-data>")
        function = _verification_function(tree, "run_bounded_git_query")
        capture_assignment = next(
            node
            for node in ast.walk(function)
            if isinstance(node, ast.Assign)
            and isinstance(node.value, ast.Call)
            and self.scanner["python_dotted_name"](node.value.func) == "capture_git_query_output"  # type: ignore[operator]
        )
        bounded_call = capture_assignment.value
        capture_assignment.value = ast.Call(
            func=ast.Attribute(
                value=ast.Name(id="process", ctx=ast.Load()),
                attr="communicate",
                ctx=ast.Load(),
            ),
            args=[],
            keywords=[],
        )
        function.body.insert(
            0,
            ast.If(
                test=ast.Constant(value=False),
                body=[ast.Expr(value=bounded_call)],
                orelse=[],
            ),
        )
        ast.fix_missing_locations(tree)
        self.assertIsNotNone(
            self.scanner["inspect_python_heredoc"](ast.unparse(tree), marker),  # type: ignore[operator]
            "a dead bounded collector must not certify an unbounded communicate result",
        )

        after_return_tree = ast.parse(bounded_body, filename="<late-query-capture-data>")
        after_return_function = _verification_function(
            after_return_tree, "run_bounded_git_query"
        )
        after_return_parents = {
            child: parent
            for parent in ast.walk(after_return_tree)
            for child in ast.iter_child_nodes(parent)
        }
        late_capture = next(
            node
            for node in ast.walk(after_return_function)
            if isinstance(node, ast.Assign)
            and isinstance(node.value, ast.Call)
            and self.scanner["python_dotted_name"](node.value.func)  # type: ignore[operator]
            == "capture_git_query_output"
        )
        completed_return = next(
            node
            for node in ast.walk(after_return_function)
            if isinstance(node, ast.Return)
            and isinstance(node.value, ast.Call)
            and self.scanner["python_dotted_name"](node.value.func)  # type: ignore[operator]
            == "subprocess.CompletedProcess"
        )
        capture_parent = after_return_parents[late_capture]
        return_parent = after_return_parents[completed_return]
        self.assertIsInstance(capture_parent, ast.Try)
        self.assertIsInstance(return_parent, ast.Try)
        capture_parent.body.remove(late_capture)  # type: ignore[union-attr]
        return_parent.body.insert(  # type: ignore[union-attr]
            return_parent.body.index(completed_return) + 1, late_capture
        )
        ast.fix_missing_locations(after_return_tree)
        self.assertIsNotNone(
            self.scanner["inspect_python_heredoc"](  # type: ignore[operator]
                ast.unparse(after_return_tree), marker
            ),
            "a collector placed after the returned result must not certify its output",
        )

    def test_git_query_process_stream_authority_cannot_be_replaced(self) -> None:
        bounded_body, marker = self.packet_heredoc_containing(
            "def run_bounded_git_query(",
            "stdout, stderr = capture_git_query_output(",
        )
        inspect = self.scanner["inspect_python_heredoc"]
        self.assertIsNone(inspect(bounded_body, marker))  # type: ignore[operator]
        mutations = (
            (
                "drop stdout before bounded capture",
                [
                    ast.If(
                        test=ast.Compare(
                            left=ast.Constant(value="status"),
                            ops=[ast.In()],
                            comparators=[ast.Name(id="command", ctx=ast.Load())],
                        ),
                        body=[
                            ast.Assign(
                                targets=[
                                    ast.Attribute(
                                        value=ast.Name(id="process", ctx=ast.Load()),
                                        attr="stdout",
                                        ctx=ast.Store(),
                                    )
                                ],
                                value=ast.Constant(value=None),
                            )
                        ],
                        orelse=[],
                    )
                ],
            ),
            (
                "save then replace stdout before bounded capture",
                [
                    ast.Assign(
                        targets=[ast.Name(id="saved_stdout", ctx=ast.Store())],
                        value=ast.Attribute(
                            value=ast.Name(id="process", ctx=ast.Load()),
                            attr="stdout",
                            ctx=ast.Load(),
                        ),
                    ),
                    ast.Assign(
                        targets=[
                            ast.Attribute(
                                value=ast.Name(id="process", ctx=ast.Load()),
                                attr="stdout",
                                ctx=ast.Store(),
                            )
                        ],
                        value=ast.Constant(value=None),
                    ),
                ],
            ),
            (
                "replace stdout through setattr before bounded capture",
                [
                    ast.Expr(
                        value=ast.Call(
                            func=ast.Name(id="setattr", ctx=ast.Load()),
                            args=[
                                ast.Name(id="process", ctx=ast.Load()),
                                ast.Constant(value="stdout"),
                                ast.Constant(value=None),
                            ],
                            keywords=[],
                        )
                    )
                ],
            ),
            (
                "nested closure replaces stdout before bounded capture",
                [
                    ast.FunctionDef(
                        name="hide_status_output",
                        args=ast.arguments(
                            posonlyargs=[],
                            args=[],
                            vararg=None,
                            kwonlyargs=[],
                            kw_defaults=[],
                            kwarg=None,
                            defaults=[],
                        ),
                        body=[
                            ast.If(
                                test=ast.Compare(
                                    left=ast.Constant(value="status"),
                                    ops=[ast.In()],
                                    comparators=[
                                        ast.Name(id="command", ctx=ast.Load())
                                    ],
                                ),
                                body=[
                                    ast.Assign(
                                        targets=[
                                            ast.Attribute(
                                                value=ast.Name(
                                                    id="process", ctx=ast.Load()
                                                ),
                                                attr="stdout",
                                                ctx=ast.Store(),
                                            )
                                        ],
                                        value=ast.Constant(value=None),
                                    )
                                ],
                                orelse=[],
                            )
                        ],
                        decorator_list=[],
                    ),
                    ast.Expr(
                        value=ast.Call(
                            func=ast.Name(
                                id="hide_status_output", ctx=ast.Load()
                            ),
                            args=[],
                            keywords=[],
                        )
                    ),
                ],
            ),
            (
                "lambda closure replaces stdout before bounded capture",
                [
                    ast.If(
                        test=ast.Compare(
                            left=ast.Constant(value="status"),
                            ops=[ast.In()],
                            comparators=[ast.Name(id="command", ctx=ast.Load())],
                        ),
                        body=[
                            ast.Expr(
                                value=ast.Call(
                                    func=ast.Lambda(
                                        args=ast.arguments(
                                            posonlyargs=[],
                                            args=[],
                                            vararg=None,
                                            kwonlyargs=[],
                                            kw_defaults=[],
                                            kwarg=None,
                                            defaults=[],
                                        ),
                                        body=ast.Call(
                                            func=ast.Name(
                                                id="setattr", ctx=ast.Load()
                                            ),
                                            args=[
                                                ast.Name(
                                                    id="process", ctx=ast.Load()
                                                ),
                                                ast.Constant(value="stdout"),
                                                ast.Constant(value=None),
                                            ],
                                            keywords=[],
                                        ),
                                    ),
                                    args=[],
                                    keywords=[],
                                )
                            )
                        ],
                        orelse=[],
                    )
                ],
            ),
        )
        for label, statements in mutations:
            with self.subTest(mutation=label):
                tree = ast.parse(bounded_body, filename="<mutable-git-pipe-data>")
                function = _verification_function(tree, "run_bounded_git_query")
                creation = next(
                    node
                    for node in ast.walk(function)
                    if isinstance(node, ast.Assign)
                    and isinstance(node.value, ast.Call)
                    and self.scanner["python_dotted_name"](node.value.func)  # type: ignore[operator]
                    == "subprocess.Popen"
                )
                parents = {
                    child: parent
                    for parent in ast.walk(function)
                    for child in ast.iter_child_nodes(parent)
                }
                block = parents[creation]
                self.assertIsInstance(block, ast.Try)
                insertion = block.body.index(creation) + 1  # type: ignore[union-attr]
                block.body[insertion:insertion] = statements  # type: ignore[union-attr]
                ast.fix_missing_locations(tree)
                self.assertIsNotNone(
                    inspect(ast.unparse(tree), marker),  # type: ignore[operator]
                    f"mutable Popen stream authority was certified: {label}",
                )

    def test_git_query_collector_must_return_bounded_pipe_bytes(self) -> None:
        collector_body, marker = self.packet_heredoc_containing(
            "def capture_git_query_output(process, git_command, output_limit, input_bytes=None):",
            'return bytes(captures["stdout"]), bytes(captures["stderr"])',
            "def run_bounded_git_query(",
        )
        inspect = self.scanner["inspect_python_heredoc"]
        self.assertIsNone(
            inspect(collector_body, marker),  # type: ignore[operator]
            "the canonical bounded pipe collector remains accepted",
        )
        mutations = (
            (
                "process.communicate() output",
                [
                    ast.Return(
                        value=ast.Call(
                            func=ast.Attribute(
                                value=ast.Name(id="process", ctx=ast.Load()),
                                attr="communicate",
                                ctx=ast.Load(),
                            ),
                            args=[],
                            keywords=[],
                        )
                    )
                ],
            ),
            (
                "fabricated origin bytes after wait",
                [
                    ast.Expr(
                        value=ast.Call(
                            func=ast.Attribute(
                                value=ast.Name(id="process", ctx=ast.Load()),
                                attr="wait",
                                ctx=ast.Load(),
                            ),
                            args=[],
                            keywords=[],
                        )
                    ),
                    ast.Return(
                        value=ast.Tuple(
                            elts=[
                                ast.Constant(
                                    value=b"https://github.com/1XP-AI/gh-runnerd.git\n"
                                ),
                                ast.Constant(value=b""),
                            ],
                            ctx=ast.Load(),
                        )
                    ),
                ],
            ),
        )
        for label, replacement in mutations:
            with self.subTest(mutant=label):
                tree = ast.parse(collector_body, filename="<git-query-collector-data>")
                collector = _verification_function(tree, "capture_git_query_output")
                collector.body = replacement
                ast.fix_missing_locations(tree)
                self.assertIsNotNone(
                    inspect(ast.unparse(tree), marker),  # type: ignore[operator]
                    label,
                )
        for label, change in (
            (
                "empty budget len operand",
                lambda guard: guard.test.left.left.left.args.clear(),
            ),
            (
                "non-call budget operand",
                lambda guard: setattr(
                    guard.test.left.left,
                    "left",
                    ast.Constant(value=0),
                ),
            ),
        ):
            with self.subTest(mutant=label):
                tree = ast.parse(collector_body, filename="<git-query-collector-data>")
                collector = _verification_function(tree, "capture_git_query_output")
                guard = next(
                    node
                    for node in ast.walk(collector)
                    if isinstance(node, ast.If)
                    and isinstance(node.test, ast.Compare)
                    and isinstance(node.test.comparators[0], ast.Name)
                    and node.test.comparators[0].id == "output_limit"
                )
                change(guard)
                ast.fix_missing_locations(tree)
                self.assertIsNotNone(
                    inspect(ast.unparse(tree), marker),  # type: ignore[operator]
                    label,
                )

        def add_fabricated_yields(
            collector: ast.FunctionDef, _drain: ast.While
        ) -> None:
            collector.body[0:0] = [
                ast.Expr(
                    value=ast.Yield(
                        value=ast.Constant(
                            value=b"https://github.com/1XP-AI/gh-runnerd.git\n"
                        )
                    )
                ),
                ast.Expr(value=ast.Yield(value=ast.Constant(value=b""))),
            ]

        control_flow_mutations = (
            (
                "conditional status break before draining output",
                lambda collector, drain: drain.body.insert(
                    0,
                    ast.If(
                        test=ast.Compare(
                            left=ast.Constant(value="status"),
                            ops=[ast.In()],
                            comparators=[ast.Name(id="git_command", ctx=ast.Load())],
                        ),
                        body=[ast.Break()],
                        orelse=[],
                    ),
                ),
            ),
            (
                "early continue before deadline and output drain",
                lambda collector, drain: drain.body.insert(
                    0,
                    ast.If(
                        test=ast.Compare(
                            left=ast.Name(id="output_limit", ctx=ast.Load()),
                            ops=[ast.Gt()],
                            comparators=[ast.Constant(value=0)],
                        ),
                        body=[ast.Continue()],
                        orelse=[],
                    ),
                ),
            ),
            (
                "additional nested loop",
                lambda collector, drain: drain.body.insert(
                    0,
                    ast.While(
                        test=ast.Constant(value=True),
                        body=[ast.Pass()],
                        orelse=[],
                    ),
                ),
            ),
            (
                "fabricated origin bytes yielded before collection",
                add_fabricated_yields,
            ),
        )
        for label, mutate in control_flow_mutations:
            with self.subTest(mutant=label):
                tree = ast.parse(collector_body, filename="<git-query-control-flow-data>")
                collector = _verification_function(tree, "capture_git_query_output")
                drain = next(
                    node
                    for node in ast.walk(collector)
                    if isinstance(node, ast.While)
                    and ast.unparse(node.test) == "selector.get_map()"
                )
                mutate(collector, drain)
                ast.fix_missing_locations(tree)
                self.assertIsNotNone(
                    inspect(ast.unparse(tree), marker),  # type: ignore[operator]
                    label,
                )
        tree = ast.parse(collector_body, filename="<git-query-collector-data>")
        collector = _verification_function(tree, "capture_git_query_output")
        collector.body.insert(
            0,
            ast.Delete(
                targets=[ast.Name(id="captures", ctx=ast.Del())],
            ),
        )
        ast.fix_missing_locations(tree)
        self.assertIsNotNone(
            inspect(ast.unparse(tree), marker),  # type: ignore[operator]
            "an unexpected local deletion shape must be rejected without crashing",
        )
        tree = ast.parse(collector_body, filename="<git-query-collector-data>")
        collector = _verification_function(tree, "capture_git_query_output")
        parents = {
            child: parent
            for parent in ast.walk(tree)
            for child in ast.iter_child_nodes(parent)
        }
        returned = next(node for node in ast.walk(collector) if isinstance(node, ast.Return))
        returned.value.elts[0].args[0].value.id = "capture_alias"
        return_parent = parents[returned]
        return_parent.body.insert(
            return_parent.body.index(returned),
            ast.Assign(
                targets=[ast.Name(id="capture_alias", ctx=ast.Store())],
                value=ast.Name(id="captures", ctx=ast.Load()),
            ),
        )
        ast.fix_missing_locations(tree)
        self.assertIsNotNone(
            inspect(ast.unparse(tree), marker),  # type: ignore[operator]
            "an output-buffer alias must not inherit the reviewed capture proof",
        )


    def test_bounded_git_origin_capture_requires_reviewed_comparison_and_no_output_escape(self) -> None:
        verification_body, verification_marker = self.packet_heredoc_containing(
            "expected_origin_url =",
            "origin_result = run_bounded_git_query(",
            "if origin_urls != [expected_origin_url]:",
        )
        inspect_verification = self.scanner["inspect_python_heredoc"]
        self.assertIsNone(
            inspect_verification(verification_body, verification_marker),  # type: ignore[operator]
            "the current bounded, captured-and-compared verifier remains accepted",
        )
        comparison_proof = self.scanner["reviewed_python_git_origin_capture_is_compared"]
        verification_tree = ast.parse(verification_body)
        verification_parents = {
            child: parent
            for parent in ast.walk(verification_tree)
            for child in ast.iter_child_nodes(parent)
        }
        self.assertTrue(
            comparison_proof(verification_tree, verification_parents)  # type: ignore[operator]
        )
        for label, mutant in (
            (
                "raw stdout output",
                verification_body + "\nprint(origin_result.stdout)\n",
            ),
            (
                "captured stdout alias output",
                verification_body
                + "\nraw_origin = origin_result.stdout\nprint(raw_origin)\n",
            ),
            (
                "decoded origin list output",
                verification_body + "\nprint(origin_urls)\n",
            ),
            (
                "origin comparison removed",
                self._verification_without_origin_comparison(
                    verification_body
                ),
            ),
        ):
            with self.subTest(mutant=label):
                mutant_tree = ast.parse(mutant)
                mutant_parents = {
                    child: parent
                    for parent in ast.walk(mutant_tree)
                    for child in ast.iter_child_nodes(parent)
                }
                self.assertFalse(
                    comparison_proof(mutant_tree, mutant_parents)  # type: ignore[operator]
                )
                self.assertIsNotNone(
                    inspect_verification(mutant, verification_marker),  # type: ignore[operator]
                    label,
                )

    def test_git_query_runtime_dependencies_cannot_be_patched_around_capture(self) -> None:
        verification_body, marker = self.packet_heredoc_containing(
            "expected_origin_url =",
            "origin_result = run_bounded_git_query(",
            "if origin_urls != [expected_origin_url]:",
        )
        inspect_verification = self.scanner["inspect_python_heredoc"]
        self.assertIsNone(
            inspect_verification(verification_body, marker),  # type: ignore[operator]
            "the original imported collector dependencies remain accepted",
        )
        origin_bytes = b"https://github.com/1XP-AI/gh-runnerd.git\n"
        self.assertTrue(origin_bytes.endswith(b"\n"))
        synthetic_code = '''
class SyntheticSelectorKey:
    def __init__(self, stream, data):
        self.fileobj = stream
        self.data = data

class SyntheticSelector:
    def __init__(self):
        self.entries = []
    def register(self, stream, _event, data):
        self.entries.append(SyntheticSelectorKey(stream, data))
    def get_map(self):
        return self.entries
    def select(self, _timeout):
        return [(self.entries[0], 1)] if self.entries else []
    def unregister(self, stream):
        self.entries = [entry for entry in self.entries if entry.fileobj is not stream]
    def close(self):
        self.entries = []

synthetic_origin = __ORIGIN_BYTES__
synthetic_chunks = iter((synthetic_origin, b""))
def synthetic_read(_fd, _size):
    return next(synthetic_chunks, b"")
'''
        synthetic_tree = ast.parse(synthetic_code, filename="<collector-dependency-data>")
        placeholders = [
            node for node in ast.walk(synthetic_tree)
            if isinstance(node, ast.Name)
            and node.id == "__ORIGIN_BYTES__"
            and isinstance(node.ctx, ast.Load)
        ]
        self.assertEqual(1, len(placeholders))
        synthetic_origin_assignment = next(
            node for node in synthetic_tree.body
            if isinstance(node, ast.Assign)
            and any(
                isinstance(target, ast.Name) and target.id == "synthetic_origin"
                for target in node.targets
            )
        )
        synthetic_origin_assignment.value = ast.Constant(value=origin_bytes)
        self.assertEqual(
            origin_bytes,
            synthetic_origin_assignment.value.value,
            "the inert replacement reader must yield the actual newline-terminated origin bytes",
        )
        tree = ast.parse(verification_body, filename="<git-origin-verifier-data>")
        origin_query = next(
            statement for statement in tree.body
            if isinstance(statement, ast.Assign)
            and any(
                isinstance(target, ast.Name) and target.id == "origin_result"
                for target in statement.targets
            )
            and isinstance(statement.value, ast.Call)
            and ast.unparse(statement.value.func) == "run_bounded_git_query"
        )
        comparison = next(
            node for node in ast.walk(tree)
            if isinstance(node, ast.If)
            and isinstance(node.test, ast.Compare)
            and any(
                isinstance(part, ast.Name) and part.id == "origin_urls"
                for part in ast.walk(node.test)
            )
        )
        query_index = tree.body.index(origin_query)
        tree.body[query_index:query_index] = synthetic_tree.body
        query_index = tree.body.index(origin_query)
        patch = ast.parse(
            "saved_default_selector = selectors.DefaultSelector\n"
            "saved_os_read = os.read\n"
            "selectors.DefaultSelector = SyntheticSelector\n"
            "os.read = synthetic_read\n"
        ).body
        tree.body[query_index:query_index] = patch
        comparison_index = tree.body.index(comparison)
        restore = ast.parse(
            "selectors.DefaultSelector = saved_default_selector\n"
            "os.read = saved_os_read\n"
        ).body
        tree.body[comparison_index + 1:comparison_index + 1] = restore
        ast.fix_missing_locations(tree)
        self.assertIsNotNone(
            inspect_verification(ast.unparse(tree), marker),  # type: ignore[operator]
            "saved, patched, and restored selector/read callables must not certify Git output",
        )

        aliased = ast.parse(verification_body, filename="<git-provider-alias-data>")
        alias_query = next(
            statement for statement in aliased.body
            if isinstance(statement, ast.Assign)
            and any(
                isinstance(target, ast.Name) and target.id == "origin_result"
                for target in statement.targets
            )
        )
        alias_comparison = next(
            node for node in ast.walk(aliased)
            if isinstance(node, ast.If)
            and isinstance(node.test, ast.Compare)
            and any(
                isinstance(part, ast.Name) and part.id == "origin_urls"
                for part in ast.walk(node.test)
            )
        )
        alias_query_index = aliased.body.index(alias_query)
        aliased.body[alias_query_index:alias_query_index] = ast.parse(
            "selector_provider = selectors\n"
            "reader_provider = os\n"
            "process_provider = subprocess\n"
            "saved_selector = selector_provider.DefaultSelector\n"
            "saved_reader = reader_provider.read\n"
            "saved_popen = process_provider.Popen\n"
            "selector_provider.DefaultSelector = SyntheticSelector\n"
            "reader_provider.read = synthetic_read\n"
            "process_provider.Popen = replacement_popen\n"
        ).body
        alias_comparison_index = aliased.body.index(alias_comparison)
        aliased.body[alias_comparison_index + 1:alias_comparison_index + 1] = ast.parse(
            "selector_provider.DefaultSelector = saved_selector\n"
            "reader_provider.read = saved_reader\n"
            "process_provider.Popen = saved_popen\n"
        ).body
        ast.fix_missing_locations(aliased)
        self.assertIsNotNone(
            inspect_verification(ast.unparse(aliased), marker),  # type: ignore[operator]
            "module aliases and saved launcher callables must not certify Git output",
        )

        reflected = ast.parse(
            verification_body, filename="<git-provider-reflection-data>"
        )
        reflected_query = next(
            statement for statement in reflected.body
            if isinstance(statement, ast.Assign)
            and any(
                isinstance(target, ast.Name) and target.id == "origin_result"
                for target in statement.targets
            )
        )
        reflected_comparison = next(
            node for node in ast.walk(reflected)
            if isinstance(node, ast.If)
            and isinstance(node.test, ast.Compare)
            and any(
                isinstance(part, ast.Name) and part.id == "origin_urls"
                for part in ast.walk(node.test)
            )
        )
        reflected_index = reflected.body.index(reflected_query)
        reflected.body[reflected_index:reflected_index] = ast.parse(
            "provider_namespace = globals()\n"
            "selector_provider = provider_namespace['selectors']\n"
            "reader_provider = provider_namespace['os']\n"
            "saved_selector = selector_provider.DefaultSelector\n"
            "saved_reader = reader_provider.read\n"
            "selector_provider.DefaultSelector = SyntheticSelector\n"
            "reader_provider.read = synthetic_read\n"
        ).body
        reflected_comparison_index = reflected.body.index(reflected_comparison)
        reflected.body[reflected_comparison_index + 1:reflected_comparison_index + 1] = ast.parse(
            "selector_provider.DefaultSelector = saved_selector\n"
            "reader_provider.read = saved_reader\n"
        ).body
        ast.fix_missing_locations(reflected)
        self.assertIsNotNone(
            inspect_verification(ast.unparse(reflected), marker),  # type: ignore[operator]
            "global provider lookup must not bypass stable imported-module bindings",
        )

        shadowed_builtin = ast.parse(
            verification_body, filename="<git-provider-builtin-data>"
        )
        builtin_query = next(
            statement for statement in shadowed_builtin.body
            if isinstance(statement, ast.Assign)
            and any(
                isinstance(target, ast.Name) and target.id == "origin_result"
                for target in statement.targets
            )
        )
        shadowed_builtin.body.insert(
            shadowed_builtin.body.index(builtin_query),
            ast.parse("SystemExit = RuntimeError").body[0],
        )
        ast.fix_missing_locations(shadowed_builtin)
        self.assertIsNotNone(
            inspect_verification(ast.unparse(shadowed_builtin), marker),  # type: ignore[operator]
            "rebinding the budget guard's builtin must not certify Git output",
        )

        reexported = ast.parse(
            verification_body, filename="<git-private-provider-reexport-data>"
        )
        reexport_query = next(
            statement for statement in reexported.body
            if isinstance(statement, ast.Assign)
            and any(
                isinstance(target, ast.Name) and target.id == "origin_result"
                for target in statement.targets
            )
        )
        reexport_patch = ast.parse(
            f"synthetic_chunks = iter(({origin_bytes!r}, b''))\n"
            "def synthetic_read(_fd, _size):\n"
            "    return next(synthetic_chunks, b'')\n"
            "__import__('tempfile')._os.read = synthetic_read\n",
            filename="<inert-private-provider-patch-data>",
        )
        reexported.body[
            reexported.body.index(reexport_query):reexported.body.index(reexport_query)
        ] = reexport_patch.body
        ast.fix_missing_locations(reexported)
        with self.subTest(mutated_provider="dynamic tempfile OS re-export"):
            self.assertIsNotNone(
                inspect_verification(ast.unparse(reexported), marker),  # type: ignore[operator]
                "a dynamic tempfile OS re-export must not supply the bounded Git pipe bytes",
            )

        for helper, replacement in (
            (
                "run_bounded_git_query",
                f"run_bounded_git_query = lambda *args, **kwargs: "
                f"subprocess.CompletedProcess([], 0, {origin_bytes!r}, b'')\n",
            ),
            (
                "capture_git_query_output",
                "capture_git_query_output = lambda *args: (b'', b'')\n",
            ),
            (
                "terminate_git_query_group",
                "terminate_git_query_group = lambda process: None\n",
            ),
            (
                "close_git_query_streams",
                "close_git_query_streams = lambda process: None\n",
            ),
            (
                "wait_for_git_query_group_exit",
                "wait_for_git_query_group_exit = lambda process, deadline: True\n",
            ),
            (
                "git_query_group_exists",
                "git_query_group_exists = lambda process: False\n",
            ),
            (
                "git_query",
                "git_query = lambda arguments: []\n",
            ),
        ):
            with self.subTest(rebound_git_helper=helper):
                mutated = ast.parse(
                    verification_body, filename="<git-helper-rebinding-data>"
                )
                origin_query = next(
                    statement for statement in mutated.body
                    if isinstance(statement, ast.Assign)
                    and any(
                        isinstance(target, ast.Name) and target.id == "origin_result"
                        for target in statement.targets
                    )
                )
                insertion = mutated.body.index(origin_query)
                mutated.body[insertion:insertion] = ast.parse(
                    replacement, filename="<inert-helper-rebinding-data>"
                ).body
                ast.fix_missing_locations(mutated)
                self.assertIsNotNone(
                    inspect_verification(ast.unparse(mutated), marker),  # type: ignore[operator]
                    f"rebinding {helper} must not inherit the reviewed collector proof",
                )

        for label, pattern_binding in (
            (
                "match capture",
                "match 0:\n"
                "    case run_bounded_git_query:\n"
                "        pass\n",
            ),
            (
                "nested mapping capture",
                'match {"query": {"runner": 1}}:\n'
                '    case {"query": {"runner": run_bounded_git_query}}:\n'
                "        pass\n",
            ),
        ):
            with self.subTest(pattern_bound_git_helper=label):
                mutated = ast.parse(
                    verification_body, filename="<git-helper-pattern-binding-data>"
                )
                origin_query = next(
                    statement for statement in mutated.body
                    if isinstance(statement, ast.Assign)
                    and any(
                        isinstance(target, ast.Name) and target.id == "origin_result"
                        for target in statement.targets
                    )
                )
                insertion = mutated.body.index(origin_query)
                mutated.body[insertion:insertion] = ast.parse(
                    pattern_binding, filename="<inert-helper-pattern-binding-data>"
                ).body
                ast.fix_missing_locations(mutated)
                self.assertIsNotNone(
                    inspect_verification(ast.unparse(mutated), marker),  # type: ignore[operator]
                    f"pattern capture {label} must not replace the reviewed Git runner",
                )

        for label, namespace_write in (
            (
                "function globals attribute",
                "synthetic_runner.__globals__['run_bounded_git_query'] = synthetic_runner\n",
            ),
            (
                "reflected function globals attribute",
                "getattr(synthetic_runner, '__globals__')['run_bounded_git_query'] = synthetic_runner\n",
            ),
        ):
            with self.subTest(git_helper_namespace_write=label):
                mutated = ast.parse(
                    verification_body, filename="<git-helper-namespace-write-data>"
                )
                origin_query = next(
                    statement for statement in mutated.body
                    if isinstance(statement, ast.Assign)
                    and any(
                        isinstance(target, ast.Name) and target.id == "origin_result"
                        for target in statement.targets
                    )
                )
                insertion = mutated.body.index(origin_query)
                synthetic_runner = (
                    "def synthetic_runner(*args, **kwargs):\n"
                    f"    return subprocess.CompletedProcess([], 0, {origin_bytes!r}, b'')\n"
                    + namespace_write
                )
                mutated.body[insertion:insertion] = ast.parse(
                    synthetic_runner,
                    filename="<inert-git-helper-namespace-write-data>",
                ).body
                ast.fix_missing_locations(mutated)
                self.assertIsNotNone(
                    inspect_verification(ast.unparse(mutated), marker),  # type: ignore[operator]
                    f"{label} must not redirect the verified origin query",
                )

    @staticmethod
    def _verification_without_origin_comparison(body: str) -> str:
        mutated = ast.parse(body, filename="<verification-output-data>")
        comparisons = [
            statement
            for statement in mutated.body
            if isinstance(statement, ast.If)
            and isinstance(statement.test, ast.Compare)
            and any(
                isinstance(node, ast.Name) and node.id == "origin_urls"
                for node in ast.walk(statement.test)
            )
        ]
        if len(comparisons) != 1:
            raise AssertionError("current origin comparison shape changed")
        mutated.body.remove(comparisons[0])
        return ast.unparse(mutated)

    def test_explicit_executable_paths_require_reviewed_locations(self) -> None:
        for command in (
            "/tmp/git status --porcelain=v1",
            "./git status --porcelain=v1",
        ):
            with self.subTest(command=command):
                self.assertIsNotNone(self.shell_violation(command))
        self.assertIsNone(
            self.shell_violation(_isolated_git_shell_command(["status", "--porcelain=v1"]))
        )

    def test_git_config_assignments_cannot_replace_reviewed_fence_state(self) -> None:
        commands = (
            "GIT_CONFIG_COUNT=1\n"
            "GIT_CONFIG_KEY_0=diff.external\n"
            "GIT_CONFIG_VALUE_0=/tmp/reviewed-hook\n"
            "git diff HEAD^ HEAD"
        )
        self.assertIsNotNone(self.shell_document_violation(commands))

    def test_git_show_output_and_reader_option_paths_are_reviewed(self) -> None:
        self.assertIsNotNone(
            self.shell_violation(
                "git show --output=AGENTS.md --format=oneline -s HEAD"
            )
        )
        self.assertIsNotNone(
            self.shell_violation(
                "diff --from-file=$HOME/.netrc docs/EXECUTION.md"
            )
        )

    def test_git_diff_output_cannot_replace_reviewed_source(self) -> None:
        for command in (
            "git diff --output=AGENTS.md HEAD^ HEAD",
            "git diff --output AGENTS.md HEAD^ HEAD",
            "git log --output=AGENTS.md -1",
            "git show --output AGENTS.md HEAD",
        ):
            with self.subTest(command=command):
                self.assertIsNotNone(self.shell_violation(command))
        self.assertIsNone(self.shell_violation(_isolated_git_shell_command(["diff", "HEAD^", "HEAD"])))

    def test_nested_raise_does_not_prove_module_root_guard(self) -> None:
        bodies = [
            body
            for _line, body, _safe_marker, _invocation
            in self.scanner["python_heredoc_bodies"](PACKET_TEXT)  # type: ignore[operator]
            if "def source_fuzz_guard():" in body
        ]
        self.assertEqual(len(bodies), 1)
        original = (
            'if module_dir != "experiments/g01-scaleset":\n'
            '    raise SystemExit(f"{label}: unexpected module directory {module_dir!r}")'
        )
        unreachable = (
            'if module_dir != "experiments/g01-scaleset":\n'
            '    if False:\n'
            '        raise SystemExit(f"{label}: unexpected module directory {module_dir!r}")'
        )
        body_tree = ast.parse(bodies[0], filename="<module-root-guard-specimen>")
        guards = [
            statement
            for statement in body_tree.body
            if isinstance(statement, ast.If)
            and isinstance(statement.test, ast.Compare)
            and isinstance(statement.test.left, ast.Name)
            and statement.test.left.id == "module_dir"
        ]
        self.assertEqual(len(guards), 1)
        source_lines = bodies[0].splitlines(keepends=True)
        start = sum(len(line) for line in source_lines[: guards[0].lineno - 1])
        end = sum(len(line) for line in source_lines[: guards[0].end_lineno])
        guard_source = bodies[0][start:end]
        mutated_guard = guard_source.replace(original, unreachable, 1)
        self.assertNotEqual(mutated_guard, guard_source)
        mutated = bodies[0][:start] + mutated_guard + bodies[0][end:]
        self.assertFalse(self.package_directory_guard_is_reviewed(mutated))

    def test_unreachable_raise_does_not_prove_package_path_containment(self) -> None:
        bodies = [
            body
            for _line, body, _safe_marker, _invocation
            in self.scanner["python_heredoc_bodies"](PACKET_TEXT)  # type: ignore[operator]
            if "def source_fuzz_guard():" in body
        ]
        self.assertEqual(len(bodies), 1)
        original = (
            '    except ValueError:\n'
            '        raise SystemExit(f"{label}: package source escaped the reviewed module")'
        )
        unreachable = (
            '    except ValueError:\n'
            '        return\n'
            '        raise SystemExit(f"{label}: package source escaped the reviewed module")'
        )
        mutated = self.replace_source_fuzz_guard_fragment(
            bodies[0], original, unreachable
        )
        self.assertFalse(self.package_directory_guard_is_reviewed(mutated))

    def test_unreachable_package_containment_try_is_not_reviewed(self) -> None:
        bodies = [
            body
            for _line, body, _safe_marker, _invocation
            in self.scanner["python_heredoc_bodies"](PACKET_TEXT)  # type: ignore[operator]
            if "def source_fuzz_guard():" in body
        ]
        self.assertEqual(len(bodies), 1)
        original = (
            '    try:\n'
            '        package_dir.relative_to(go_repo_root / module_dir)\n'
            '    except ValueError:\n'
            '        raise SystemExit(f"{label}: package source escaped the reviewed module")'
        )
        self.assertIsNone(self.inspect(bodies[0]))
        unreachable_variants = (
            (
                '    if False:\n'
                '        try:\n'
                '            package_dir.relative_to(go_repo_root / module_dir)\n'
                '        except ValueError:\n'
                '            raise SystemExit(f"{label}: package source escaped the reviewed module")'
            ),
            (
                '    if True:\n'
                '        pass\n'
                '    else:\n'
                '        try:\n'
                '            package_dir.relative_to(go_repo_root / module_dir)\n'
                '        except ValueError:\n'
                '            raise SystemExit(f"{label}: package source escaped the reviewed module")'
            ),
            (
                '    if 0:\n'
                '        try:\n'
                '            package_dir.relative_to(go_repo_root / module_dir)\n'
                '        except ValueError:\n'
                '            raise SystemExit(f"{label}: package source escaped the reviewed module")'
            ),
            (
                '    if module_dir == "experiments/g01-scaleset":\n'
                '        pass\n'
                '    else:\n'
                '        try:\n'
                '            package_dir.relative_to(go_repo_root / module_dir)\n'
                '        except ValueError:\n'
                '            raise SystemExit(f"{label}: package source escaped the reviewed module")'
            ),
            (
                '    if not False:\n'
                '        pass\n'
                '    else:\n'
                '        try:\n'
                '            package_dir.relative_to(go_repo_root / module_dir)\n'
                '        except ValueError:\n'
                '            raise SystemExit(f"{label}: package source escaped the reviewed module")'
            ),
            (
                '    try:\n'
                '        package_dir.relative_to(go_repo_root / module_dir)\n'
                '    except Exception:\n'
                '        pass\n'
                '    except ValueError:\n'
                '        raise SystemExit(f"{label}: package source escaped the reviewed module")'
            ),
        )
        for unreachable in unreachable_variants:
            with self.subTest(unreachable=unreachable):
                mutated = self.replace_source_fuzz_guard_fragment(
                    bodies[0], original, unreachable
                )
                self.assertIsNotNone(self.inspect(mutated))

    def test_package_containment_try_requires_direct_reachable_body(self) -> None:
        bodies = [
            body
            for _line, body, _safe_marker, _invocation
            in self.scanner["python_heredoc_bodies"](PACKET_TEXT)  # type: ignore[operator]
            if "def source_fuzz_guard():" in body
        ]
        self.assertEqual(len(bodies), 1)
        original = (
            '    try:\n'
            '        package_dir.relative_to(go_repo_root / module_dir)\n'
            '    except ValueError:\n'
            '        raise SystemExit(f"{label}: package source escaped the reviewed module")'
        )
        self.assertIsNone(self.inspect(bodies[0]))
        conditional_variants = (
            (
                '    if 0 == 1:\n'
                '        try:\n'
                '            package_dir.relative_to(go_repo_root / module_dir)\n'
                '        except ValueError:\n'
                '            raise SystemExit(f"{label}: package source escaped the reviewed module")'
            ),
            (
                '    if 1 == 1:\n'
                '        pass\n'
                '    else:\n'
                '        try:\n'
                '            package_dir.relative_to(go_repo_root / module_dir)\n'
                '        except ValueError:\n'
                '            raise SystemExit(f"{label}: package source escaped the reviewed module")'
            ),
        )
        for replacement in conditional_variants:
            with self.subTest(replacement=replacement):
                mutated = self.replace_source_fuzz_guard_fragment(
                    bodies[0], original, replacement
                )
                self.assertFalse(self.package_directory_guard_is_reviewed(mutated))

    def test_packet_loader_rejects_packet_controlled_definition_time_code(self) -> None:
        specimens = (
            "import synthetic_side_effect\n",
            "@synthetic_side_effect()\ndef scanner():\n    return None\n",
            "def scanner(value=synthetic_side_effect()):\n    return value\n",
            "def set():\n    return None\n",
            "def ast():\n    return None\n",
            "set = lambda: None\n",
            "import re\nre.compile = synthetic_side_effect\n",
            "def scanner(*args: synthetic_side_effect()):\n    return None\n",
            "def scanner(**kwargs: synthetic_side_effect()):\n    return None\n",
        )
        for source in specimens:
            with self.subTest(source=source):
                module = ast.parse(source, filename="<loader-specimen-data>")
                with self.assertRaises(AssertionError):
                    _validated_scanner_statements(module)

    def test_packet_loader_rejects_unsupported_top_level_assignment(self) -> None:
        original = globals()["PACKET_TEXT"]
        try:
            unsupported = original.replace(
                "def inspect_python_heredoc(body, safe_marker):",
                'probe = os.system("gh workflow run ci.yml")\n'
                'def inspect_python_heredoc(body, safe_marker):',
                1,
            )
            self.assertNotEqual(unsupported, original)
            scanner_source = _scanner_module_source(unsupported)
            self.assertIn('probe = os.system("gh workflow run ci.yml")', scanner_source)
            globals()["PACKET_TEXT"] = unsupported
            self.assertNotEqual(globals()["PACKET_TEXT"], original)
            with self.assertRaisesRegex(AssertionError, "unsupported top-level assignment"):
                _scanner_namespace()

            scanner_source = _scanner_module_source(original)
            module = ast.parse(scanner_source, filename="<current-packet-scanner-data>")
            source_statement = _top_level_assignment(module, "source")
            source_expression = ast.get_source_segment(scanner_source, source_statement)
            self.assertEqual(
                source_expression,
                'source = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")',
            )
            self.assertIsInstance(source_statement, ast.Assign)
            self.assertTrue(_validated_scanner_statements(module))
            source_position = original.find(scanner_source)
            self.assertGreaterEqual(source_position, 0)
            self.assertEqual(original.find(scanner_source, source_position + 1), -1)
            line_offsets = [0]
            for line in scanner_source.splitlines(keepends=True):
                line_offsets.append(line_offsets[-1] + len(line))
            start = line_offsets[source_statement.lineno - 1] + source_statement.col_offset
            end = line_offsets[source_statement.end_lineno - 1] + source_statement.end_col_offset
            mutated_scanner_source = (
                scanner_source[:start]
                + 'source = os.system("gh workflow run ci.yml")'
                + scanner_source[end:]
            )
            mutated_packet = (
                original[:source_position]
                + mutated_scanner_source
                + original[source_position + len(scanner_source):]
            )
            self.assertEqual(_scanner_module_source(mutated_packet), mutated_scanner_source)
            globals()["PACKET_TEXT"] = mutated_packet
            with self.assertRaisesRegex(AssertionError, "source assignment is not reviewed"):
                _scanner_namespace()
        finally:
            globals()["PACKET_TEXT"] = original

    def test_packet_loader_rejects_bare_top_level_expression(self) -> None:
        module = ast.parse('print(subprocess.os.environ)\n')
        with self.assertRaises(AssertionError):
            _validated_scanner_statements(module)
        self.assertIsNotNone(self.inspect('import subprocess\nprint(subprocess.os.environ)\n'))

    def test_reexported_os_module_does_not_bypass_heredoc_checks(self) -> None:
        unsafe = (
            'import subprocess\nsubprocess.os.remove("/tmp/maintainer-owned")\n',
            'import subprocess as sp\nsp.os.remove("/tmp/maintainer-owned")\n',
            'from subprocess import os as operating\noperating.remove("/tmp/maintainer-owned")\n',
            'import subprocess\nsp = subprocess\nsp.os.remove("/tmp/maintainer-owned")\n',
            'import subprocess\ngetattr(subprocess, "os").remove("/tmp/maintainer-owned")\n',
            'import subprocess\nsubprocess.__dict__["os"].remove("/tmp/maintainer-owned")\n',
            'import subprocess\nother, = (subprocess,)\nother.os.remove("/tmp/maintainer-owned")\n',
            'import subprocess\nother = [subprocess][0]\nother.os.remove("/tmp/maintainer-owned")\n',
            'import subprocess\nlookup = getattr\nlookup(subprocess, "os").remove("/tmp/maintainer-owned")\n',
            'import subprocess\nlookup = vars\nlookup(subprocess)["os"].remove("/tmp/maintainer-owned")\n',
            'import subprocess\nother, = (subprocess,)\nlookup = getattr\nmember = "os"\nlookup(other, member).system("gh workflow run ci.yml")\n',
            'import subprocess\nother = {"module": subprocess}["module"]\nlookup = getattr\nmember = "os"\nlookup(other, member).system("gh workflow run ci.yml")\n',
            'import subprocess\nclass Holder:\n    pass\nholder = Holder()\nholder.module = subprocess\nother = holder.module\nlookup = getattr\nmember = "os"\nlookup(other, member).system("gh workflow run ci.yml")\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect('import subprocess\nprint("reviewed")\n'))
        self.assertIsNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'value = namespace["safe"]\nnamespace["safe"] = value\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess}\n'
            'other = namespace["subprocess"]\ngetattr(other, "os").system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'namespace.update({"safe": subprocess})\n'
            'other = namespace["safe"]\ngetattr(other, "os").system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'namespace.update({"safe": subprocess})\nother = namespace["safe"]\n'
            'lookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"safe": subprocess}\n'
            'if False:\n    namespace = {"safe": "reviewed", "subprocess": subprocess}\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"safe": "reviewed", "subprocess": subprocess}\n'
            'namespace, = ({"safe": subprocess},)\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"safe": "reviewed", "subprocess": subprocess}\n'
            'for namespace in [{"safe": subprocess}]:\n    other = namespace["safe"]\n'
            'lookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'update = namespace.update\nupdate({"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'dict.update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'mapping_type = dict\nmapping_type.update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'getattr(dict, "update")(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'type(namespace).update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'namespace.__class__.update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'type([namespace][0]).update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            '[namespace][0].__class__.update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'getattr([namespace][0], "__class__").update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'namespace.copy().__class__.update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'update = dict.__dict__["update"]\nupdate(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'update = vars(dict)["update"]\nupdate(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'dict.__mro__[0].update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'type({}).update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            '{}.__class__.update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'type("D", (dict,), {}).update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNone(self.inspect(
            'import subprocess\n'
            'fake_os = type("FakeOS", (), {"environ": {"fixture": "reviewed"}})\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'dict_type = type.__new__(type, "D", (dict,), {})\n'
            'dict_type.update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'def type(*args):\n    return dict\n'
            'type("FakeOS", (), {"environ": {"fixture": "reviewed"}}).update('
            'namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'getattr(type, "__new__")(type, "D", (dict,), {}).update('
            'namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nnamespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'vars(type)["__new__"](type, "D", (dict,), {}).update('
            'namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nclass D(dict):\n    pass\n'
            'namespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'D.update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNone(self.inspect(
            'import subprocess\nclass StopAtChild(Exception):\n    pass\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\nException = dict\nclass D(Exception):\n    pass\n'
            'namespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'D.update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess, sys\nsys._getframe().f_globals["Exception"] = dict\n'
            'class D(Exception):\n    pass\n'
            'namespace = {"subprocess": subprocess, "safe": "reviewed"}\n'
            'D.update(namespace, {"safe": subprocess})\n'
            'other = namespace["safe"]\nlookup = getattr\nmember = "os"\n'
            'lookup(other, member).system("gh workflow run ci.yml")\n'
        ))
        self.assertIsNone(self.inspect(
            'import subprocess\n'
            'class Settings:\n    os = "darwin"\n'
            'print(Settings.os)\n'
        ))

    def test_command_capable_decorator_cannot_replace_safe_function(self) -> None:
        body = (
            'import subprocess\n'
            'def deco(function):\n'
            '    return subprocess.run\n'
            '@deco\n'
            'def launch(command):\n'
            '    return None\n'
            'launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        self.assertIsNotNone(self.inspect(body))
        self.assertIsNotNone(self.inspect(
            'import subprocess\n'
            'def sm(function):\n    return subprocess.run\n'
            'class C:\n    @sm\n    def launch(command):\n        return None\n'
            'C().launch(["gh", "workflow", "run", "ci.yml"])\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import subprocess\n'
            'class GoAliasPopen:\n'
            '    @property\n'
            '    def returncode(self):\n'
            '        return subprocess.run\n'
        ))
        for mutation in (
            '__builtins__["property"] = replace\n',
            '__builtins__.property = replace\n',
            'setattr(__builtins__, "property", replace)\n',
            'ns = globals()["__builtins__"]\n'
            'if isinstance(ns, dict):\n    ns["property"] = replace\n'
            'else:\n    ns.property = replace\n',
        ):
            body = (
                'import subprocess\n'
                'def replace(function):\n    return subprocess.run\n'
                + mutation
                + 'class GoAliasPopen:\n'
                '    @property\n'
                '    def returncode(self):\n'
                '        return self._process.returncode\n'
                'GoAliasPopen.returncode(["gh", "workflow", "run", "ci.yml"])\n'
            )
            with self.subTest(mutation=mutation):
                self.assertIsNotNone(self.inspect(body))
        for acquisition in (
            'ns = locals()["__builtins__"]\n',
            'ns = vars()["__builtins__"]\n',
            'import sys\nns = sys.modules["builtins"]\n',
            'from sys import modules\nns = modules["builtins"]\n',
            'from sys import *\nns = modules["builtins"]\n',
            'lookup, = (globals,)\nns = lookup()["__builtins__"]\n',
        ):
            body = (
                'import subprocess\n'
                'def replace(function):\n    return subprocess.run\n'
                + acquisition
                + 'if isinstance(ns, dict):\n    ns["property"] = replace\n'
                'else:\n    ns.property = replace\n'
                'class GoAliasPopen:\n'
                '    @property\n'
                '    def returncode(self):\n'
                '        return self._process.returncode\n'
                'GoAliasPopen.returncode(["gh", "workflow", "run", "ci.yml"])\n'
            )
            with self.subTest(acquisition=acquisition):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect('def reviewed():\n    return "safe"\nprint(reviewed())\n'))

    def test_output_sink_method_alias_keeps_sensitive_taint(self) -> None:
        unsafe = (
            'import os, sys\nemit = sys.stdout.write\nemit(str(os.environ))\n',
            'import os, sys\nemit = sys.stdout.write\nagain = emit\nagain(str(os.environ))\n',
            'import os, sys\nsinks = {"emit": sys.stdout.write}\nsinks["emit"](str(os.environ))\n',
            'import os, sys\nsinks = [sys.stdout.write]\nsinks[0](str(os.environ))\n',
            'import os, sys\nlookup = getattr\nemit = lookup(sys.stdout, "write")\nemit(str(os.environ))\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect(
            'import sys\nemit = sys.stdout.write\nemit("reviewed")\n'
        ))

    def test_filesystem_mutator_cannot_hide_in_container_binding(self) -> None:
        unsafe = (
            'import os\nactions = {"delete": os.remove}\nactions["delete"]("/tmp/maintainer-owned")\n',
            'import os\nactions = [os.remove]\nactions[0]("/tmp/maintainer-owned")\n',
            'from os import remove as erase\nactions = {"delete": erase}\nactions["delete"]("/tmp/maintainer-owned")\n',
            'import os as operating\nactions = {"delete": operating.remove}\nactions["delete"]("/tmp/maintainer-owned")\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

    def test_constructor_and_output_sink_aliases_preserve_sensitive_taint(self) -> None:
        unsafe = (
            'import os\n'
            'maker = dict\n'
            'value = maker(os.environ)\n'
            'print(value)\n',
            'import os\n'
            'emit = print\n'
            'emit(os.environ)\n',
            'import os\n'
            'import warnings\n'
            'emit = warnings.warn\n'
            'emit(os.environ)\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'maker = dict\n'
            'print(maker({"status": "ready"}))\n',
            'emit = print\n'
            'emit("reviewed")\n',
            'import warnings\n'
            'emit = warnings.warn\n'
            'emit("reviewed")\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_assigned_sensitive_constructor_aliases_preserve_taint(self) -> None:
        unsafe = (
            'import os\nmaker = list\nvalue = maker(os.environ)\nprint(value)\n',
            'import os\nmaker = tuple\nvalue = maker(os.environ)\nprint(value)\n',
            'import os\nmaker = set\nvalue = maker(os.environ)\nprint(value)\n',
            'import os\nmaker = str\nvalue = maker(os.environ)\nprint(value)\n',
            'import os\nmaker = repr\nvalue = maker(os.environ)\nprint(value)\n',
            'import os\nmaker = bytes\n'
            'value = maker(next(iter(os.environ.values()), "").encode())\n'
            'print(value)\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'maker = list\nvalue = maker(["status: reviewed"])\nprint(value)\n',
            'maker = tuple\nvalue = maker(("status: reviewed",))\nprint(value)\n',
            'maker = set\nvalue = maker({"status: reviewed"})\nprint(value)\n',
            'maker = str\nvalue = maker("status: reviewed")\nprint(value)\n',
            'maker = repr\nvalue = maker("status: reviewed")\nprint(value)\n',
            'maker = bytes\nvalue = maker(b"status: reviewed")\nprint(value)\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_home_and_decoded_local_paths_are_not_disclosed(self) -> None:
        unsafe = (
            'from pathlib import Path\nprint(Path.home())\n',
            'from pathlib import Path as P\nprint(P.home())\n',
            'import pathlib as pl\nprint(pl.Path.home())\n',
            'from pathlib import Path\nhome = Path.home\nprint(home())\n',
            'from pathlib import Path\nprint(Path("~").expanduser())\n',
            'import os\nprint(os.path.expanduser("~"))\n',
            'import os\nfrom pathlib import Path\n'
            'print(os.fsdecode(Path.cwd().resolve()))\n',
            'import os\nhome = os.environ["HOME"]\nprint(home)\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'from pathlib import Path\n'
            'root = Path.home()\n'
            'if not root.is_absolute():\n'
            '    raise SystemExit("invalid home root")\n',
            'from pathlib import Path as P\n'
            'print(P("docs/evidence/g01-recovery-packet.md"))\n',
            'import pathlib as pl\n'
            'print(pl.Path("docs/evidence/g01-recovery-packet.md"))\n',
            'from pathlib import Path as P\n'
            'is_absolute = P("docs/evidence/g01-recovery-packet.md").is_absolute\n'
            'if is_absolute():\n    raise SystemExit("unexpected absolute path")\n',
            'from pathlib import Path\n'
            'print(Path("docs/evidence/g01-recovery-packet.md").expanduser())\n',
            'import os\nprint(os.path.expanduser("docs/evidence/g01-recovery-packet.md"))\n',
            'import os\nfrom pathlib import Path\n'
            'print(os.fsdecode(Path("docs/evidence/g01-recovery-packet.md").as_posix().encode()))\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_current_directory_path_aliases_are_not_disclosed(self) -> None:
        unsafe = (
            'from pathlib import Path as P\nprint(P.cwd())\n',
            'import pathlib as pl\nprint(pl.Path.cwd())\n',
            'from pathlib import Path\ncwd = Path.cwd\nprint(cwd())\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'from pathlib import Path\n'
            'cwd = Path.cwd\n'
            'if not cwd().is_absolute():\n'
            '    raise SystemExit("invalid working root")\n',
            'from pathlib import Path as P\n'
            'print(P("docs/evidence/g01-recovery-packet.md"))\n',
            'import pathlib as pl\n'
            'print(pl.Path("docs/evidence/g01-recovery-packet.md"))\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_contextlib_chdir_is_rejected_through_aliases_and_containers(self) -> None:
        unsafe = (
            'import contextlib\n'
            'with contextlib.chdir("synthetic-other-root"):\n'
            '    print(open("docs/backlog.json").read())\n',
            'from contextlib import chdir as switch_root\n'
            'with switch_root("synthetic-other-root"):\n'
            '    print(open("docs/backlog.json").read())\n',
            'import contextlib as cwd_tools\n'
            'switch_root = cwd_tools.chdir\n'
            'with switch_root("synthetic-other-root"):\n'
            '    print(open("docs/backlog.json").read())\n',
            'import contextlib as cwd_tools\n'
            'actions = {"root": cwd_tools.chdir}\n'
            'with actions["root"]("synthetic-other-root"):\n'
            '    print(open("docs/backlog.json").read())\n',
            'import contextlib as cwd_tools\n'
            'actions = [cwd_tools.chdir]\nalias = actions\n'
            'with alias[0]("synthetic-other-root"):\n'
            '    print(open("docs/backlog.json").read())\n',
            'import contextlib\nmodules = {"context": contextlib}\n'
            'with modules["context"].chdir("synthetic-other-root"):\n'
            '    print(open("docs/backlog.json").read())\n',
            'from os import chdir as switch_root\n'
            'with switch_root("synthetic-other-root"):\n'
            '    print(open("docs/backlog.json").read())\n',
            'with __import__("contextlib").chdir("synthetic-other-root"):\n'
            '    print(open("docs/backlog.json").read())\n',
            'module = __import__("contextlib")\n'
            'with module.chdir("synthetic-other-root"):\n'
            '    print(open("docs/backlog.json").read())\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect(
            'import contextlib\nwith contextlib.nullcontext():\n    pass\n'
        ))

    def test_path_method_alias_chains_respect_lexical_shadowing(self) -> None:
        unsafe = (
            'from pathlib import Path\n'
            'home = Path.home\n'
            'other = home\n'
            'print(other())\n',
            'from pathlib import Path\n'
            'cwd = Path.cwd\n'
            'other = cwd\n'
            'print(other())\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'from pathlib import Path\n'
            'home = Path.home\n'
            'def report(home):\n'
            '    print(home())\n',
            'from pathlib import Path\n'
            'cwd = Path.cwd\n'
            'def report(cwd):\n'
            '    print(cwd())\n',
            'from pathlib import Path\n'
            'home = other\n'
            'other = home\n'
            'print(other())\n',
            'from pathlib import Path\n'
            'print(Path("docs/evidence/g01-recovery-packet.md"))\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_path_method_aliases_follow_defaults_getattr_and_namedexpr(self) -> None:
        unsafe = (
            'from pathlib import Path as P\n'
            'def report(home=P.home):\n'
            '    print(home())\n'
            'report()\n',
            'from pathlib import Path as P\n'
            'get = getattr\n'
            'home = get(P, "home")\n'
            'print(home())\n',
            'from pathlib import Path as P\n'
            'print((home := P.home)())\n',
            'from pathlib import Path as P\n'
            'print((cwd := getattr(P, "cwd"))())\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'from pathlib import Path as P\n'
            'print(P("docs/evidence/g01-recovery-packet.md"))\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_path_home_alias_reassignment_and_helpers_remain_safe(self) -> None:
        safe = (
            'from pathlib import Path as P\n'
            'home = P.home\n'
            'home = lambda: "reviewed"\n'
            'print(home())\n',
            'from pathlib import Path as P\n'
            'home = P.home\n'
            'home = lambda: "reviewed"\n'
            'def report():\n'
            '    return home()\n'
            'print(report())\n',
            'from pathlib import Path as P\n'
            'home = P.home\n'
            'home = lambda: "reviewed"\n'
            'class Report:\n'
            '    def home(self):\n'
            '        return "reviewed"\n'
            'print(Report().home())\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_path_home_alias_conditional_reassignment_retains_taint(self) -> None:
        body = (
            'from pathlib import Path as P\n'
            'home = P.home\n'
            'if False:\n'
            '    home = lambda: "reviewed"\n'
            'print(home())\n'
        )
        self.assertIsNotNone(self.inspect(body))

    def test_named_expression_callable_sink_preserves_environment_taint(self) -> None:
        self.assertIsNotNone(self.inspect('import os\n(emit := print)(os.environ)\n'))
        self.assertIsNone(self.inspect('emit = print\nemit("status: reviewed")\n'))

    def test_named_expression_sink_alias_chain_preserves_environment_taint(self) -> None:
        unsafe = (
            'import os\n'
            'emit = print\n'
            '(alias := emit)(os.environ)\n',
            'import os\n'
            '(alias := print)(os.environ)\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'emit = print\n'
            '(alias := emit)({"status": "reviewed"})\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_bash_indirect_environment_expansion_rejects_credential_names(self) -> None:
        self.assertIsNotNone(self.shell_violation('name=GH_TOKEN; printf "%s\\n" "${!name}"'))
        self.assertIsNone(self.shell_violation('printf "%s\\n" "status: reviewed"'))

    def test_shell_credential_assignment_aliases_are_rejected(self) -> None:
        unsafe = (
            "secret=$GH_TOKEN\nprintf '%s\\n' $secret",
            "secret=$GH_TOKEN\ncopy=$secret\nprintf '%s\\n' $copy",
            "secret=$GH_TOKEN\n[ 1 = 2 ] && secret=reviewed\nprintf '%s\\n' \"$secret\"",
        )
        for commands in unsafe:
            with self.subTest(commands=commands):
                self.assertIsNotNone(self.shell_document_violation(commands))

        for commands in (
            "secret=reviewed\nprintf '%s\\n' $secret",
            "secret=$GH_TOKEN\nsecret=reviewed\nprintf '%s\\n' $secret",
        ):
            with self.subTest(commands=commands):
                self.assertIsNone(self.shell_document_violation(commands))

    def test_conditional_and_env_prefix_assignments_do_not_clear_shell_taint(self) -> None:
        commands_by_case = {
            "conditional-if": (
                "secret=$GH_TOKEN\n"
                "if [ 1 = 2 ]; then secret=reviewed; fi\n"
                "printf '%s\\n' \"$secret\""
            ),
            "env-prefix": (
                "secret=$GH_TOKEN\n"
                "env secret=reviewed printf '%s\\n' \"$secret\""
            ),
        }
        for label, commands in commands_by_case.items():
            with self.subTest(label=label):
                markdown = f"```sh\n{commands}\n```\n"
                output_violation = None
                for command, _line in self.scanner["shell_commands"](markdown):  # type: ignore[operator]
                    for segment in self.scanner["shell_token_segments"](command):  # type: ignore[operator]
                        if "printf" in segment:
                            output_violation = self.scanner["forbidden_shell_command"](segment)  # type: ignore[operator]
                self.assertIsNotNone(output_violation)

    def test_shell_parameter_modifier_keeps_sensitive_assignment_taint(self) -> None:
        commands = (
            "secret=${GH_TOKEN#x}\n"
            "printf '%s\\n' \"$secret\""
        )
        markdown = f"```sh\n{commands}\n```\n"
        output_violation = None
        for command, _line in self.scanner["shell_commands"](markdown):  # type: ignore[operator]
            for segment in self.scanner["shell_token_segments"](command):  # type: ignore[operator]
                if segment and segment[0] == "printf":
                    output_violation = self.scanner["forbidden_shell_command"](segment)  # type: ignore[operator]
        self.assertIsNotNone(output_violation)

    def test_shell_indirect_environment_expansion_in_assignment_is_rejected(self) -> None:
        self.assertIsNotNone(
            self.shell_document_violation(
                "name=GH_TOKEN\nsecret=${!name}\nprintf '%s\\n' \"$secret\""
            )
        )

    def test_shell_environment_dump_readers_are_rejected(self) -> None:
        for command in (
            'awk \'BEGIN { print ENVIRON["GH_TOKEN"] }\'',
            'awk \'BEGIN { for (name in ENVIRON) print ENVIRON[name] }\'',
            'jq -n env',
            'jq -n \'env.GH_TOKEN\'',
        ):
            with self.subTest(command=command):
                self.assertIsNotNone(self.shell_violation(command))

        for command in (
            'awk \'BEGIN { print "reviewed" }\'',
            'jq -n \'"reviewed"\'',
        ):
            with self.subTest(command=command):
                self.assertIsNone(self.shell_violation(command))

    def test_os_module_assignment_alias_cannot_hide_filesystem_mutation(self) -> None:
        self.assertIsNotNone(self.inspect(
            'import os\nalias = os\nalias.remove("/tmp/maintainer-owned")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import os\nalias, = (os,)\nalias.remove("/tmp/maintainer-owned")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import os\nfor alias in (os,):\n    alias.remove("synthetic-maintainer-owned")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import os\nfor alias in tuple([os]):\n    alias.remove("synthetic-maintainer-owned")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import os\nfor alias in iter([os]):\n    alias.remove("synthetic-maintainer-owned")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import os\nfor alias in filter(None, [os]):\n'
            '    alias.remove("synthetic-maintainer-owned")\n'
        ))
        self.assertIsNotNone(self.inspect('import os\nalias = os\nprint(alias.environ)\n'))
        self.assertIsNotNone(self.inspect(
            'import os\nalias = os\nprint(alias.getenv("GH_TOKEN"))\n'
        ))
        self.assertIsNone(self.inspect('import os\nalias = os\nprint("reviewed")\n'))

    def test_from_import_environment_alias_remains_sensitive(self) -> None:
        self.assertIsNotNone(self.inspect(
            'from os import environ as inherited\nprint(inherited)\n'
        ))
        self.assertIsNone(self.inspect(
            'from os import environ as inherited\nprint("reviewed")\n'
        ))

    def test_byte_environment_mappings_are_sensitive(self) -> None:
        for body in (
            'import os\nprint(os.environb)\n',
            'from os import environb\nprint(environb)\n',
            'from os import getenvb as read_bytes\nprint(read_bytes(b"GH_TOKEN"))\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

    def test_environment_reader_alias_index_is_reused_for_an_immutable_tree(self) -> None:
        tree = ast.parse('import os\nreader = os.getenvb\nsecond = reader\n')
        original_walk = ast.walk
        walks = []
        def counted_walk(node):
            walks.append(id(node))
            return original_walk(node)
        try:
            ast.walk = counted_walk
            first = self.scanner["python_credential_reader_aliases"](tree)
            self.assertEqual({"reader", "second"}, set(first))
            count = len(walks)
            second = self.scanner["python_credential_reader_aliases"](tree)
            self.assertEqual(first, second)
            self.assertEqual(count, len(walks), "unchanged AST must not rebuild the reader alias index")
        finally:
            ast.walk = original_walk

    def test_os_reexports_from_allowed_modules_are_not_certified(self) -> None:
        for body in (
            'import tempfile\ntempfile._os.remove("synthetic-owned")\n',
            'import tempfile as fixtures\nfixtures._os.remove("synthetic-owned")\n',
            'from tempfile import _os as inherited\ninherited.remove("synthetic-owned")\n',
            'import pathlib\npathlib.os.remove("synthetic-owned")\n',
            'import pathlib as files\nfiles.os.remove("synthetic-owned")\n',
            'import pathlib\nmodules = {"pathlib": pathlib}\n'
            'module = modules["pathlib"]\nmodule.os.remove("synthetic-owned")\n',
            'import pathlib\ngetattr(pathlib, "os").remove("synthetic-owned")\n',
            'import tempfile\ngetattr(tempfile, "_os").remove("synthetic-owned")\n',
            'import tempfile\ngetattr(tempfile, "_" + "os").remove("synthetic-owned")\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect('import tempfile\nprint("offline fixture")\n'))

    def test_displayhook_aliases_are_sensitive_output_sinks(self) -> None:
        for body in (
            'import os, sys\nsys.displayhook(os.environ)\n',
            'import os, sys\nemit = sys.displayhook\nemit(os.environ)\n',
            'import os\nfrom sys import displayhook as emit\nemit(os.environ)\n',
            'import os\nimport sys as runtime\nruntime.displayhook(os.environ)\n',
            'import os, sys\nruntime = sys\nemit = runtime.displayhook\nemit(os.environ)\n',
            'import os, sys\n(emit := sys.displayhook)(os.environ)\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect('import sys\nsys.displayhook("offline fixture")\n'))
        self.assertIsNone(self.inspect('from sys import displayhook as emit\nemit("offline fixture")\n'))

    def test_excepthook_aliases_are_sensitive_error_sinks(self) -> None:
        unsafe = (
            'import os, sys\n'
            'sys.excepthook(RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nfrom sys import excepthook as emit\n'
            'emit(RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nfrom sys import excepthook as source\n'
            'emit = (tmp := source)\n'
            'emit(RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nfrom sys import excepthook as source\n'
            'emit = ((tmp := source),)[0]\n'
            'emit(RuntimeError, RuntimeError(os.environ), None)\n',
            'import os, sys\nemit = sys.excepthook\n'
            'emit(RuntimeError, RuntimeError(os.environ), None)\n',
            'import os, sys\nemit = getattr(sys, "excepthook")\n'
            'emit(RuntimeError, RuntimeError(os.environ), None)\n',
            'import os, sys\nruntime = sys\nlookup = getattr\n'
            'emit = lookup(runtime, "excepthook")\n'
            'emit(RuntimeError, RuntimeError(os.environ), None)\n',
            'import os, sys\n'
            'getattr(sys, "excepthook")(RuntimeError, RuntimeError(os.environ), None)\n',
            'import os, sys\nruntime = sys\n'
            'runtime.excepthook(RuntimeError, RuntimeError(os.environ), None)\n',
            'import os, sys\nhooks = [sys.excepthook]\n'
            'hooks[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os, sys\nhooks = (sys.excepthook,)\n'
            'hooks[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os, sys\nhooks = {"error": sys.excepthook}\n'
            'hooks["error"](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os, sys\nhooks = [sys.excepthook]\nhandlers = hooks\n'
            'handlers[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nfrom sys import excepthook as emit\n'
            'handlers = [emit]\n'
            'handlers[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nfrom sys import excepthook as emit\n'
            'handlers = (emit,)\n'
            'handlers[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nfrom sys import excepthook as emit\n'
            'handlers = {"error": emit}\n'
            'handlers["error"](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os, sys\nemit = sys.excepthook\nhandlers = [emit]\n'
            'handlers[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os, sys\nemit = getattr(sys, "excepthook")\n'
            'handlers = [emit]\n'
            'handlers[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nfrom sys import excepthook as emit\n'
            'def unrelated(emit):\n    pass\n'
            'handlers = [emit]\n'
            'handlers[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nfrom sys import excepthook as emit\n'
            'def unrelated():\n    def emit():\n        pass\n'
            'handlers = [emit]\n'
            'handlers[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nfrom sys import excepthook as emit\n'
            'def unrelated():\n    class emit:\n        pass\n'
            'handlers = [emit]\n'
            'handlers[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nfrom sys import excepthook as emit\n'
            'def unrelated(emit):\n    pass\n'
            'emit(RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nglobal emit\n'
            'from sys import excepthook as emit\n'
            'emit(RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nglobal emit\n'
            'from sys import excepthook as emit\n'
            'handlers = [emit]\n'
            'handlers[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\n'
            'def install():\n'
            '    global emit\n'
            '    from sys import excepthook as emit\n'
            '    emit(RuntimeError, RuntimeError(os.environ), None)\n'
            'install()\n',
            'import os\n'
            'def install():\n'
            '    global emit\n'
            '    from sys import excepthook as emit\n'
            '    handlers = [emit]\n'
            '    handlers[0](RuntimeError, RuntimeError(os.environ), None)\n'
            'install()\n',
            'import os\n'
            'def outer():\n'
            '    emit = None\n'
            '    def install():\n'
            '        nonlocal emit\n'
            '        from sys import excepthook as emit\n'
            '        emit(RuntimeError, RuntimeError(os.environ), None)\n'
            '    install()\n'
            'outer()\n',
            'import os\n'
            'def outer():\n'
            '    emit = None\n'
            '    def install():\n'
            '        nonlocal emit\n'
            '        from sys import excepthook as emit\n'
            '        handlers = [emit]\n'
            '        handlers[0](RuntimeError, RuntimeError(os.environ), None)\n'
            '    install()\n'
            'outer()\n',
            'import os\nfrom sys import excepthook as source\n'
            'def install():\n'
            '    global saved\n'
            '    saved = source\n'
            'install()\n'
            'handlers = [saved]\n'
            'handlers[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nfrom sys import excepthook as source\n'
            'def outer():\n'
            '    saved = None\n'
            '    def install():\n'
            '        nonlocal saved\n'
            '        saved = source\n'
            '    install()\n'
            '    handlers = [saved]\n'
            '    handlers[0](RuntimeError, RuntimeError(os.environ), None)\n'
            'outer()\n',
            'import os\n'
            'def outer():\n'
            '    from sys import excepthook as emit\n'
            '    def inner():\n'
            '        nonlocal emit\n'
            '        emit(RuntimeError, RuntimeError(os.environ), None)\n'
            '    inner()\n'
            'outer()\n',
            'import os\n'
            'def outer():\n'
            '    from sys import excepthook as emit\n'
            '    def inner():\n'
            '        nonlocal emit\n'
            '        handlers = [emit]\n'
            '        handlers[0](RuntimeError, RuntimeError(os.environ), None)\n'
            '    inner()\n'
            'outer()\n',
            'import os\n'
            'def outer():\n'
            '    emit = None\n'
            '    def middle():\n'
            '        def install():\n'
            '            nonlocal emit\n'
            '            from sys import excepthook as emit\n'
            '        install()\n'
            '    middle()\n'
            '    handlers = [emit]\n'
            '    handlers[0](RuntimeError, RuntimeError(os.environ), None)\n'
            'outer()\n',
            'import os\nfrom sys import excepthook as emit\n'
            'saved = emit\nemit = None\nhandlers = [saved]\n'
            'handlers[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nfrom sys import excepthook as source\n'
            'def outer():\n'
            '    saved = None\n'
            '    def middle():\n'
            '        def install():\n'
            '            nonlocal saved\n'
            '            saved = source\n'
            '        install()\n'
            '    middle()\n'
            '    handlers = [saved]\n'
            '    handlers[0](RuntimeError, RuntimeError(os.environ), None)\n'
            'outer()\n',
            'import os\nfrom sys import excepthook as emit\n'
            'def outer():\n'
            '    def emit(*args):\n'
            '        pass\n'
            '    def inner():\n'
            '        global emit\n'
            '        handlers = [emit]\n'
            '        handlers[0](RuntimeError, RuntimeError(os.environ), None)\n'
            '    inner()\n'
            'outer()\n',
            'import os\nfrom sys import excepthook as source\n'
            'def install():\n'
            '    [emit := source for _ in (0,)]\n'
            '    emit(RuntimeError, RuntimeError(os.environ), None)\n'
            'install()\n',
            'import os\nfrom sys import excepthook as source\n'
            'def install():\n'
            '    global saved\n'
            '    [saved := source for _ in (0,)]\n'
            'install()\n'
            'handlers = [saved]\n'
            'handlers[0](RuntimeError, RuntimeError(os.environ), None)\n',
            'import os\nfrom sys import excepthook as source\n'
            'def outer():\n'
            '    saved = None\n'
            '    def install():\n'
            '        nonlocal saved\n'
            '        [saved := source for _ in (0,)]\n'
            '    install()\n'
            '    handlers = [saved]\n'
            '    handlers[0](RuntimeError, RuntimeError(os.environ), None)\n'
            'outer()\n',
            'import os\nfrom sys import excepthook as source\n'
            'def outer():\n'
            '    saved = None\n'
            '    def install():\n'
            '        nonlocal saved\n'
            '        [[saved := source for _ in (0,)] for __ in (0,)]\n'
            '    install()\n'
            '    handlers = [saved]\n'
            '    handlers[0](RuntimeError, RuntimeError(os.environ), None)\n'
            'outer()\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect(
            'import sys\nsys.excepthook(RuntimeError, RuntimeError("offline fixture"), None)\n'
        ))
        self.assertIsNone(self.inspect(
            'from sys import excepthook as emit\n'
            'emit(RuntimeError, RuntimeError("offline fixture"), None)\n'
        ))
        self.assertIsNone(self.inspect(
            'def callback(*args):\n    return None\n'
            'callbacks = [callback]\ncallbacks[0]("offline fixture")\n'
        ))
        self.assertIsNone(self.inspect(
            'from sys import excepthook as emit\n'
            'def local_callback(emit):\n'
            '    handlers = [emit]\n    handlers[0]("offline fixture")\n'
        ))
        self.assertIsNone(self.inspect(
            'def callback(*args):\n    return None\n'
            'def install():\n'
            '    [handler := callback for _ in (0,)]\n'
            '    handler("offline fixture")\n'
            'install()\n'
        ))

    def test_git_query_timeout_cleanup_must_remain_reachable(self) -> None:
        bounded_body, marker = self.packet_heredoc_containing(
            "def run_bounded_git_query(",
            "stdout, stderr = capture_git_query_output(",
        )
        inspect = self.scanner["inspect_python_heredoc"]
        self.assertIsNone(inspect(bounded_body, marker))  # type: ignore[operator]
        tree = ast.parse(bounded_body, filename="<deferred-git-timeout-cleanup-data>")
        runner = _verification_function(tree, "run_bounded_git_query")
        timeout_handler = next(
            node
            for node in ast.walk(runner)
            if isinstance(node, ast.ExceptHandler)
            and isinstance(node.type, ast.Attribute)
            and self.scanner["python_dotted_name"](node.type)  # type: ignore[operator]
            == "subprocess.TimeoutExpired"
        )
        self.assertTrue(timeout_handler.body)
        cleanup = timeout_handler.body[0]
        self.assertIsInstance(cleanup, ast.Expr)
        self.assertIsInstance(cleanup.value, ast.Call)  # type: ignore[union-attr]
        timeout_handler.body[0] = ast.FunctionDef(
            name="deferred_timeout_cleanup",
            args=ast.arguments(
                posonlyargs=[],
                args=[],
                vararg=None,
                kwonlyargs=[],
                kw_defaults=[],
                kwarg=None,
                defaults=[],
            ),
            body=[cleanup],
            decorator_list=[],
        )
        ast.fix_missing_locations(tree)
        self.assertIsNotNone(
            inspect(ast.unparse(tree), marker),  # type: ignore[operator]
            "unreachable nested timeout termination must not certify process cleanup",
        )

        for cleanup_index in range(4):
            with self.subTest(unreachable_cleanup=cleanup_index):
                tree = ast.parse(
                    bounded_body,
                    filename="<conditional-git-timeout-cleanup-data>",
                )
                runner = _verification_function(tree, "run_bounded_git_query")
                parents = {
                    child: parent
                    for parent in ast.walk(tree)
                    for child in ast.iter_child_nodes(parent)
                }
                cleanup_calls = [
                    node
                    for node in ast.walk(runner)
                    if isinstance(node, ast.Call)
                    and self.scanner["python_dotted_name"](node.func)  # type: ignore[operator]
                    == "terminate_git_query_group"
                ]
                self.assertEqual(4, len(cleanup_calls))
                cleanup_statement = parents[cleanup_calls[cleanup_index]]
                self.assertIsInstance(cleanup_statement, ast.Expr)
                container = parents[cleanup_statement]
                list_field = next(
                    field
                    for field in ("body", "orelse", "finalbody")
                    if isinstance(getattr(container, field, None), list)
                    and cleanup_statement in getattr(container, field)
                )
                statements = getattr(container, list_field)
                statement_index = statements.index(cleanup_statement)
                statements[statement_index] = ast.If(
                    test=ast.Constant(value=False),
                    body=[cleanup_statement],
                    orelse=[],
                )
                ast.fix_missing_locations(tree)
                self.assertIsNotNone(
                    inspect(ast.unparse(tree), marker),  # type: ignore[operator]
                    "a cleanup hidden behind a false condition must not certify its path",
                )

    def test_current_scanner_compiler_body_remains_self_audit_compatible(self) -> None:
        scanner_body, marker = self.packet_heredoc_containing(
            "def python_import_bindings(tree):",
            "def python_local_call_return_values(call_node, tree, parents):",
        )
        self.assertIsNone(
            self.scanner["inspect_python_heredoc"](scanner_body, marker),  # type: ignore[operator]
            "the canonical scanner/compiler body must pass its own AST audit",
        )

    def test_reviewed_scanner_namespace_remains_accepted_with_cwd_guards(self) -> None:
        scanner_body_matches = [
            (body, marker)
            for _number, body, marker, _invocation in self.scanner[
                "python_heredoc_bodies"
            ](PACKET_TEXT)  # type: ignore[operator]
            if 'scanner = {"Path": Path, "ast": ast, "os": os, "re": re, "shlex": shlex}'
            in body
        ]
        self.assertTrue(scanner_body_matches, "expected the canonical scanner heredoc")
        scanner_body, marker = scanner_body_matches[0]
        self.assertIsNone(
            self.scanner["inspect_python_heredoc"](scanner_body, marker),  # type: ignore[operator]
            "the canonical reviewed scanner namespace should pass the complete body checks",
        )

    def test_new_path_mutators_require_owned_sources_and_destinations(self) -> None:
        unsafe = (
            'from pathlib import Path\nPath("synthetic-owned").copy(Path("synthetic-destination"))\n',
            'from pathlib import Path\nPath("synthetic-owned").copy_into(Path("synthetic-destination"))\n',
            'from pathlib import Path\nPath("synthetic-owned").move_into(Path("synthetic-destination"))\n',
            'from pathlib import Path\nPath("synthetic-owned").replace(Path("synthetic-destination"))\n',
            'from pathlib import Path\nPath("synthetic-owned").lchmod(0o600)\n',
            'from pathlib import Path\nseen = Path("synthetic-owned")\n'
            'seen.copy(Path("synthetic-destination"))\n',
            'from pathlib import Path\n'
            'action = Path("synthetic-owned").move_into\n'
            'action(Path("synthetic-destination"))\n',
            'from pathlib import Path\n'
            'actions = {"move": Path("synthetic-unowned-source").move_into}\n'
            'actions["move"](Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\n'
            'actions = (Path("synthetic-unowned-source").move_into,)\n'
            'actions[0](Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\n'
            'actions = [Path("synthetic-unowned-source").move_into]\n'
            'alias = actions\nalias[0](Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    Path(td).joinpath("source").copy(Path("synthetic-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    Path(td).joinpath("source").copy(target=Path("synthetic-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    Path(td).joinpath("source").copy_into(target_dir=Path("synthetic-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    Path(td).joinpath("source").move_into(target_dir=Path("synthetic-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td).joinpath("source")\n'
            '    action = source.move_into\n'
            '    action(Path("synthetic-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    move = (tmp := source.move_into)\n'
            '    move(Path("/tmp/maintainer-owned-directory"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    move = ((tmp := source.move_into),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    move = ((tmp := getattr(source, "move_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    method = "move_into"\n'
            '    move = ((tmp := getattr(source, method)),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    member = getattr\n'
            '    move = ((tmp := member(source, "move_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    move = ((tmp := getattr(source, "move" + "_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    move = ((tmp := getattr(source, method_name)),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    move = getattr(source, "move_into")\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    getattr(source, "move_into")(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    member = getattr\n'
            '    move = member(source, "move_into")\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    member = getattr\n'
            '    member(source, "move_into")(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    getters = {"get": getattr}\n'
            '    member = getters["get"]\n'
            '    move = ((tmp := member(source, "move_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    members = {"member": getattr}\n'
            '    move = ((tmp := members["member"](source, "move_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    member = getattr if True else None\n'
            '    move = ((tmp := member(source, "move_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    member = (saved := getattr)\n'
            '    move = ((tmp := member(source, "move_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    getters = (getattr,)\n'
            '    member = getters[0]\n'
            '    move = ((tmp := member(source, "move_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    getters = [getattr]\n'
            '    member = getters[0]\n'
            '    move = ((tmp := member(source, "move_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    def get_member():\n'
            '        return getattr\n'
            '    member = get_member()\n'
            '    move = ((tmp := member(source, "move_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    def invoke_member(callback, receiver, name):\n'
            '        return callback(receiver, name)\n'
            '    move = ((tmp := invoke_member(getattr, source, "move_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    def member(receiver, name):\n'
            '        return getattr(receiver, name)\n'
            '    move = ((tmp := member(source, "move_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver):\n'
            '    move = ((tmp := getattr(receiver, "move_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver):\n'
            '    member = getattr\n'
            '    move = ((tmp := member(receiver, "move_into")),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    move = ((tmp := getattr(receiver, attribute)),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td)\n'
            '    relocate(source, "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    member = getattr\n'
            '    move = ((tmp := member(receiver, attribute)),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td)\n'
            '    relocate(source, "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute="move_into"):\n'
            '    move = ((tmp := getattr(receiver, attribute)),)[0]\n'
            '    move(Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td)\n'
            '    relocate(receiver=source)\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    move = ((tmp := getattr(receiver, attribute)),)[0]\n'
            '    action = move\n'
            '    action(Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    actions = [getattr(receiver, attribute)]\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    actions = {"move": getattr(receiver, attribute)}\n'
            '    actions["move"](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    actions = (getattr(receiver, attribute),)\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    actions = []\n'
            '    actions.append(getattr(receiver, attribute))\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    actions = {}\n'
            '    actions["move"] = getattr(receiver, attribute)\n'
            '    actions["move"](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    for action in [getattr(receiver, attribute)]:\n'
            '        action(Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    actions = [getattr(receiver, attribute) for item in [None]]\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    action = getattr(receiver, attribute)\n'
            '    action.__call__(Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    getattr(receiver, attribute).__call__(Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'attribute = "read_text"\n'
            'def relocate(receiver, attribute):\n'
            '    actions = [getattr(receiver, attribute)]\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'name = "read_text"\n'
            'def relocate(receiver, attribute):\n'
            '    name = attribute\n'
            '    actions = [getattr(receiver, name)]\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    action = ((tmp := getattr(receiver, attribute)),)[0]\n'
            '    action.__call__(Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def invoke(callback, destination):\n'
            '    callback(destination)\n'
            'def relocate(receiver, attribute):\n'
            '    action = ((tmp := getattr(receiver, attribute)),)[0]\n'
            '    invoke(action, Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    def invoke(callback):\n'
            '        action = callback\n'
            '        action(Path("synthetic-unowned-destination"))\n'
            '    invoke(getattr(receiver, attribute))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    actions = {}\n'
            '    actions.update(move=getattr(receiver, attribute))\n'
            '    actions["move"](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    actions = list([getattr(receiver, attribute)])\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    wrapper = list\n'
            '    actions = wrapper([getattr(receiver, attribute)])\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    actions = []\n'
            '    insert = actions.append\n'
            '    insert(getattr(receiver, attribute))\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    def invoke(callback):\n'
            '        def captured():\n'
            '            callback(Path("synthetic-unowned-destination"))\n'
            '        captured()\n'
            '    invoke(getattr(receiver, attribute))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def invoke(*callbacks):\n'
            '    callbacks[0](Path("synthetic-unowned-destination"))\n'
            'def relocate(receiver, attribute):\n'
            '    invoke(getattr(receiver, attribute))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def invoke(**callbacks):\n'
            '    callbacks["callback"](Path("synthetic-unowned-destination"))\n'
            'def relocate(receiver, attribute):\n'
            '    invoke(**{"callback": getattr(receiver, attribute)})\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def store(*items):\n'
            '    items[0](items[1])\n'
            'def relocate(receiver, attribute):\n'
            '    actions = []\n'
            '    insert = actions.append\n'
            '    store(*(insert, getattr(receiver, attribute)))\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'actions = []\n'
            'insert = actions.append\n'
            'def store(inserter, value):\n'
            '    inserter(value)\n'
            'def relocate(receiver, attribute):\n'
            '    store(insert, getattr(receiver, attribute))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n'
            'actions[0](Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def relocate(receiver, attribute):\n'
            '    actions = []\n'
            '    insert = actions.append\n'
            '    def populate():\n'
            '        insert(getattr(receiver, attribute))\n'
            '    populate()\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def store(inserter, value):\n'
            '    inserter(value)\n'
            'def relocate(receiver, attribute):\n'
            '    actions = []\n'
            '    store(actions.append, getattr(receiver, attribute))\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def store(inserter, target, value):\n'
            '    inserter(target, value)\n'
            'def relocate(receiver, attribute):\n'
            '    actions = []\n'
            '    store(list.append, actions, getattr(receiver, attribute))\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def build(constructor, value):\n'
            '    return constructor([value])\n'
            'def relocate(receiver, attribute):\n'
            '    actions = build(list, getattr(receiver, attribute))\n'
            '    actions[0](Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'def member(receiver, attribute):\n'
            '    return getattr(receiver, attribute)\n'
            'def relocate(receiver, attribute):\n'
            '    action = ((tmp := member(receiver, attribute)),)[0]\n'
            '    action(Path("synthetic-unowned-destination"))\n'
            'with TemporaryDirectory() as td:\n'
            '    relocate(Path(td), "move_into")\n',
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    Path(td).joinpath("source").copy(**{"target": Path("synthetic-destination")})\n',
            'from pathlib import Path\n'
            'seen = {"node"}\n'
            'for seen in [Path("synthetic-unowned-source")]:\n'
            '    seen.copy(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\n'
            'seen = {"node"}\n'
            'seen, other = (Path("synthetic-unowned-source"), None)\n'
            'seen.copy(Path("synthetic-unowned-destination"))\n',
            'from pathlib import Path\n'
            'def copier(seen=None):\n'
            '    if seen is None:\n'
            '        seen = set()\n'
            '    seen.copy(Path("synthetic-unowned-destination"))\n'
            'copier(**{"seen": Path("synthetic-unowned-source")})\n',
            'from pathlib import Path\n'
            'seen = set()\n'
            'def retarget():\n'
            '    global seen\n'
            '    seen = Path("synthetic-unowned-source")\n'
            'retarget()\n'
            'seen.copy(Path("synthetic-unowned-destination"))\n',
            'unknown_receiver.copy(Path("synthetic-destination"))\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        safe = (
            'from pathlib import Path\nfrom tempfile import TemporaryDirectory\n'
            'with TemporaryDirectory() as td:\n'
            '    source = Path(td) / "source"\n'
            '    copied = Path(td) / "copied"\n'
            '    destination_dir = Path(td) / "destination"\n'
            '    source.copy(copied)\n'
            '    source.copy_into(destination_dir)\n'
            '    source.move_into(destination_dir)\n'
            '    source.replace(copied)\n'
            '    source.copy(target=copied)\n'
            '    action = source.move_into\n'
            '    action(destination_dir)\n'
            '    source.lchmod(0o600)\n'
            '    member = getattr\n'
            '    reflected_move = member(source, "move_into")\n'
            '    reflected_move(destination_dir)\n'
            '    method_name = "move_into"\n'
            '    named_move = getattr(source, method_name)\n'
            '    named_move(destination_dir)\n'
            'seen = {"ast-node"}\n'
            'seen_copy = seen.copy()\n'
            'seen_alias = seen\n'
            'seen_alias_copy = seen_alias.copy()\n'
            'wrapper = list\n'
            'actions = wrapper(["ast-node"])\n'
            'items = []\n'
            'insert = items.append\n'
            'insert("ast-node")\n'
            'def echo(value):\n'
            '    return value\n'
            'def invoke(callback):\n'
            '    def captured():\n'
            '        callback("ast-node")\n'
            '    captured()\n'
            'invoke(echo)\n'
            'def nested_shadow(receiver, attribute):\n'
            '    def invoke_shadow(callback):\n'
            '        def captured_shadow(callback):\n'
            '            callback("ast-node")\n'
            '        captured_shadow(echo)\n'
            '    invoke_shadow(getattr(receiver, attribute))\n'
            'with TemporaryDirectory() as shadow_td:\n'
            '    nested_shadow(Path(shadow_td), "move_into")\n'
            'def nested_rebind(receiver, attribute):\n'
            '    def invoke_rebind(callback):\n'
            '        def captured_rebind():\n'
            '            callback = lambda: "ok"\n'
            '            callback()\n'
            '        captured_rebind()\n'
            '    invoke_rebind(getattr(receiver, attribute))\n'
            'with TemporaryDirectory() as rebind_td:\n'
            '    nested_rebind(Path(rebind_td), "move_into")\n'
            'def build_plain(constructor, value):\n'
            '    return constructor([value])\n'
            'plain_values = build_plain(list, "ast-node")\n'
            'def store_plain(inserter, value):\n'
            '    inserter(value)\n'
            'plain_items = []\n'
            'store_plain(plain_items.append, "ast-node")\n'
            'def shadow_plain_holder():\n'
            '    plain_items = []\n'
            '    insert_plain = plain_items.append\n'
            '    store_plain(insert_plain, "ast-node")\n'
            'shadow_plain_holder()\n'
            'def echo_variadic(value):\n'
            '    return value\n'
            'def invoke_variadic(*callbacks):\n'
            '    callbacks[0]("ast-node")\n'
            'invoke_variadic(echo_variadic)\n'
            'def invoke_keywords(**callbacks):\n'
            '    callbacks["callback"]("ast-node")\n'
            'invoke_keywords(**{"callback": echo_variadic})\n'
            'def invoke_variadic_shadow(callback):\n'
            '    def captured_variadic_shadow(*callback):\n'
            '        callback[0]("ast-node")\n'
            '    captured_variadic_shadow(echo_variadic)\n'
            'def invoke_keyword_shadow(callback):\n'
            '    def captured_keyword_shadow(**callback):\n'
            '        callback["callback"]("ast-node")\n'
            '    captured_keyword_shadow(callback=echo_variadic)\n'
            'with TemporaryDirectory() as shadow_td:\n'
            '    invoke_variadic_shadow(getattr(Path(shadow_td), "move_into"))\n'
            '    invoke_keyword_shadow(getattr(Path(shadow_td), "move_into"))\n'
            'def invoke_shadow(callback):\n'
            '    def captured_shadow(callback):\n'
            '        callback("ast-node")\n'
            '    captured_shadow(echo_variadic)\n'
            'invoke_shadow(echo_variadic)\n'
        )
        self.assertIsNone(self.inspect(safe))

    def test_string_replace_result_is_not_an_extracted_path_mutator(self) -> None:
        body = (
            'token = "docs\\\\evidence"\n'
            'normalized = token.replace("\\\\", "/")\n'
            'print(normalized)\n'
        )
        self.assertIsNone(self.inspect(body))

    def test_standalone_git_queries_require_explicit_hook_isolation(self) -> None:
        unsafe = (
            "git status --short", "git -P status --short",
            "git -c core.fsmonitor=false status --short",
            "git diff --stat", "git ls-files --cached",
            "env GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null "
            "GIT_CONFIG_SYSTEM=/dev/null git -P -c core.fsmonitor=false "
            "-c core.hooksPath=/dev/null status --short",
            "env -i PATH=/usr/bin:/bin GIT_CONFIG_NOSYSTEM=1 "
            "GIT_CONFIG_GLOBAL=/dev/null git -P -c core.fsmonitor=false "
            "-c core.hooksPath=/dev/null status --short",
            "env -i PATH=/usr/bin:/bin GIT_CONFIG_NOSYSTEM=1 "
            "GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null "
            "git -P -c core.fsmonitor=false status --short",
            "env -i PATH=/usr/bin:/bin GIT_CONFIG_NOSYSTEM=1 "
            "GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null "
            "git -P -c core.hooksPath=/dev/null status --short",
            "env -i PATH=/usr/bin:/bin GIT_CONFIG_NOSYSTEM=1 "
            "GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null "
            "git -P -c core.fsmonitor=false -c core.hooksPath=/dev/null "
            "-c core.fsmonitor=/synthetic/hook status --short",
        )
        for command in unsafe:
            with self.subTest(command=command):
                self.assertIsNotNone(self.shell_violation(command))
        for args in (
            ["status", "--short"],
            ["diff", "--stat"],
            ["ls-files", "--cached"],
        ):
            with self.subTest(args=args):
                self.assertIsNone(self.shell_violation(_isolated_git_shell_command(args)))
        self.assertIsNotNone(
            self.shell_document_violation(
                "export PATH=/usr/bin:/bin\n"
                "git -P -c core.fsmonitor=false -c core.hooksPath=/dev/null status --short"
            )
        )

    def test_current_executable_recipes_keep_git_and_path_provenance(self) -> None:
        recipes = (
            (
                "current bounded Go wrapper Git queries",
                (
                    "def git_source_control_entries(repo_root, module_dir, env):",
                    "def git_command(arguments):",
                    "package_initialization_guard()",
                ),
            ),
            (
                "current Markdown link audit",
                (
                    'files = subprocess.check_output(',
                    "def anchors(markdown):",
                    "source_path.read_text(encoding=\"utf-8\")",
                ),
            ),
            (
                "current synthetic Git hook control",
                (
                    'marker = root / "fsmonitor-invoked"',
                    'guarded = git_command([',
                    '"core.hooksPath=/dev/null", *arguments',
                    'print("GREEN focused packet regression:',
                ),
            ),
        )
        for label, fragments in recipes:
            with self.subTest(recipe=label):
                body, marker = self.packet_heredoc_containing(*fragments)
                self.assertIsNone(
                    self.scanner["inspect_python_heredoc"](body, marker),  # type: ignore[operator]
                    label,
                )

    def test_current_scanner_self_audit_ast_parameters_are_not_callbacks(self) -> None:
        body, _marker = self.packet_heredoc_containing(
            "def python_local_function_candidates(",
            "def inspect_python_heredoc(body, safe_marker):",
            "matches = []",
        )
        tree = ast.parse(body, filename="<current-packet-scanner-data>")
        helper = next(
            node
            for node in tree.body
            if isinstance(node, ast.FunctionDef)
            and node.name == "python_local_function_candidates"
        )
        source = ast.get_source_segment(body, helper)
        self.assertIsNotNone(source)
        modules, functions, unresolved = self.scanner["python_import_bindings"](tree)  # type: ignore[operator]
        self.assertFalse(unresolved)
        helper_tree = ast.Module(body=[helper], type_ignores=[])
        self.assertIsNone(
            self.scanner["python_callback_command_violation"](
                helper_tree, modules, functions
            )  # type: ignore[operator]
        )

    def test_current_vet_mutation_recipe_git_fixture_is_isolated(self) -> None:
        body, marker = self.packet_heredoc_containing(
            "reviewed_tree = subprocess.check_output(",
            "git_env = {",
            "GREEN 4005139662:",
        )
        self.assertIsNone(
            self.scanner["inspect_python_heredoc"](body, marker),  # type: ignore[operator]
            "current isolated synthetic Git/vet recipe must remain executable",
        )

    def test_markdown_source_paths_cannot_escape_git_list_origin(self) -> None:
        body, marker = self.packet_heredoc_containing(
            'files = subprocess.check_output(',
            'def anchors(markdown):',
            'source_path.read_text(encoding="utf-8")',
        )
        inspect = self.scanner["inspect_python_heredoc"]
        self.assertIsNone(inspect(body, marker))  # type: ignore[operator]
        mutations = {
            "direct append": body.replace(
                'repository_root = Path.cwd().resolve()\n',
                'files.append("experiments/g01-scaleset/.env")\n'
                'repository_root = Path.cwd().resolve()\n',
                1,
            ),
            "saved-list append": body.replace(
                'repository_root = Path.cwd().resolve()\n',
                'files_alias = files\n'
                'files_alias.append("experiments/g01-scaleset/.env")\n'
                'repository_root = Path.cwd().resolve()\n',
                1,
            ),
            "nested mutator": body.replace(
                'repository_root = Path.cwd().resolve()\n',
                'def append_untracked_path():\n'
                '    files.append("experiments/g01-scaleset/.env")\n'
                'append_untracked_path()\n'
                'repository_root = Path.cwd().resolve()\n',
                1,
            ),
            "nested repository-root retarget": body.replace(
                'for name in files:\n',
                'def retarget_repository_root():\n'
                '    global repository_root\n'
                '    repository_root = Path("synthetic-private").resolve()\n'
                'retarget_repository_root()\n'
                'for name in files:\n',
                1,
            ),
            "annotated repository-root retarget": body.replace(
                'for name in files:\n',
                'repository_root: Path = Path("synthetic-private").resolve()\n'
                'for name in files:\n',
                1,
            ),
            "augmented repository-root retarget": body.replace(
                'for name in files:\n',
                'repository_root /= Path("/tmp/synthetic-private")\n'
                'for name in files:\n',
                1,
            ),
            "function-definition repository-root retarget": body.replace(
                'for name in files:\n',
                'def repository_root():\n'
                '    return Path("synthetic-private")\n'
                'for name in files:\n',
                1,
            ),
            "import-alias repository-root retarget": body.replace(
                'for name in files:\n',
                'import pathlib as repository_root\n'
                'for name in files:\n',
                1,
            ),
            "source read inside symlink rejection branch": body.replace(
                '    if source.is_symlink():\n'
                '        errors.append(f"{name}: tracked Markdown path was a symlink")\n'
                '        continue\n',
                '',
                1,
            ).replace(
                '    markdown = "\\n".join(markdown_outside_fences(source_path.read_text(encoding="utf-8")))\n',
                '    if source.is_symlink():\n'
                '        markdown = "\\n".join(markdown_outside_fences(source_path.read_text(encoding="utf-8")))\n'
                '        errors.append(f"{name}: tracked Markdown path was a symlink: {markdown}")\n'
                '        continue\n'
                '    markdown = "\\n".join(markdown_outside_fences(source_path.read_text(encoding="utf-8")))\n',
                1,
            ),
            "loop variable rebound": body.replace(
                '    source = Path(name)\n',
                '    name = "experiments/g01-scaleset/.env"\n'
                '    source = Path(name)\n',
                1,
            ),
            "lexical guard unreachable": body.replace(
                '    if source.is_absolute() or ".." in source.parts:\n'
                '        errors.append(f"{name}: tracked Markdown path was not relative")\n'
                '        continue\n',
                '    if False:\n'
                '        if source.is_absolute() or ".." in source.parts:\n'
                '            errors.append(f"{name}: tracked Markdown path was not relative")\n'
                '            continue\n',
                1,
            ),
            "symlink guard unreachable": body.replace(
                '    if source.is_symlink():\n'
                '        errors.append(f"{name}: tracked Markdown path was a symlink")\n'
                '        continue\n',
                '    if False:\n'
                '        if source.is_symlink():\n'
                '            errors.append(f"{name}: tracked Markdown path was a symlink")\n'
                '            continue\n',
                1,
            ),
            "symlink continue does not dominate": body.replace(
                '    if source.is_symlink():\n'
                '        errors.append(f"{name}: tracked Markdown path was a symlink")\n'
                '        continue\n',
                '    if source.is_symlink():\n'
                '        if False:\n'
                '            errors.append(f"{name}: tracked Markdown path was a symlink")\n'
                '            continue\n',
                1,
            ),
        }
        for label, source in mutations.items():
            with self.subTest(mutation=label):
                self.assertNotEqual(body, source, f"failed to construct {label} witness")
                self.assertIsNotNone(
                    inspect(source, marker),  # type: ignore[operator]
                    f"Markdown read lost Git-list provenance after {label}",
                )

    def test_unbound_anchor_reader_cannot_read_or_disclose_paths(self) -> None:
        self.assertIsNotNone(self.inspect(
            'from pathlib import Path\n'
            'def anchors(path):\n'
            '    return path.read_text(encoding="utf-8")\n'
            'print(anchors(Path("synthetic-private/file")))\n'
        ))

    def test_historical_unsafe_git_transcripts_are_inert_source_text(self) -> None:
        historical_sections = (
            "The four focused offline selector checks below are historical list-only source",
            "block is the historical pre-isolation package-init audit",
            "The red extraction and assertions were run from the immutable parent with a",
            "The three red probes ran first against the exact immutable parent.",
            "The red probe ran first against the exact immutable parent. It used only the\nparent's static scanner text",
            "The focused green probe uses fake Go Popen objects backed by synthetic pipes",
            "The red probes ran first against immutable parent\n`3f6de0b227e4b44aa3d5e259e937e7dc1f0856bb`",
        )
        for section in historical_sections:
            with self.subTest(section=section):
                section_start = PACKET_TEXT.index(section)
                fence_start = PACKET_TEXT.index("```", section_start)
                self.assertTrue(
                    PACKET_TEXT.startswith("```text", fence_start),
                    "historical executable examples must be visibly inert source text",
                )

    def test_python_git_queries_require_isolated_environment_and_known_builder(self) -> None:
        unsafe = (
            'import subprocess\n'
            'subprocess.run(["git", "-P", "-c", "core.fsmonitor=false", '
            '"-c", "core.hooksPath=/dev/null", "status"])\n',
            'import subprocess\n'
            'git_environment = {"GIT_CONFIG_NOSYSTEM": "1"}\n'
            'subprocess.run(["git", "-P", "-c", "core.fsmonitor=false", '
            '"-c", "core.hooksPath=/dev/null", "status"], env=git_environment)\n',
            'import subprocess\n'
            'git_environment = {"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", '
            '"GIT_CONFIG_SYSTEM": "/dev/null", "GIT_ATTR_NOSYSTEM": "1", '
            '"GIT_CONFIG_COUNT": "2", "GIT_CONFIG_KEY_0": "core.fsmonitor", '
            '"GIT_CONFIG_VALUE_0": "false", "GIT_CONFIG_KEY_1": "core.hooksPath", '
            '"GIT_CONFIG_VALUE_1": "/dev/null"}\n'
            'subprocess.run(["git", "-P", "-c", "core.fsmonitor=false", '
            '"-c", "core.hooksPath=/dev/null", "status"], env=git_environment)\n',
            'import subprocess\n'
            'git_environment = {"PATH": "/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM": "1", '
            '"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null"}\n'
            'def git_command(arguments):\n    return ["git", *arguments]\n'
            'subprocess.run(git_command(["status"]), env=git_environment)\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        canonical = (
            'import subprocess\n'
            'git_environment = {"PATH": "/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM": "1", '
            '"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null", '
            '"GIT_ATTR_NOSYSTEM": "1"}\n'
            'def git_command(arguments):\n'
            '    return ["/usr/bin/env", "-i", "GIT_CONFIG_NOSYSTEM=1", '
            '"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", '
            '"GIT_ATTR_NOSYSTEM=1", "/usr/bin/git", "--no-replace-objects", '
            '"-P", "-c", "core.fsmonitor=false", "-c", '
            '"core.hooksPath=/dev/null", *arguments]\n'
            'subprocess.run(git_command(["status", "--short"]), env=git_environment)\n'
        )
        self.assertIsNone(self.inspect(canonical))
        direct_call = 'subprocess.run(git_command(["status", "--short"]), env=git_environment)'
        self.assertIsNone(
            self.inspect(
                canonical.replace(
                    direct_call,
                    'def query(env):\n'
                    '    subprocess.run(git_command(["status", "--short"]), env=env)\n'
                    'query(git_environment)',
                )
            )
        )
        self.assertIsNotNone(
            self.inspect(
                canonical.replace(
                    direct_call,
                    'def query():\n'
                    '    subprocess.run(git_command(["status", "--short"]), env=git_environment)\n'
                    'query()',
                )
            )
        )
        self.assertIsNotNone(
            self.inspect(canonical.rsplit("subprocess.run(", 1)[0]
                         + 'subprocess.run(git_command(["status", "--short"]))\n')
        )
        self.assertIsNotNone(
            self.inspect(
                canonical.replace(
                    '"GIT_ATTR_NOSYSTEM": "1"}',
                    '"GIT_ATTR_NOSYSTEM": "1", "GIT_DIR": "/synthetic"}',
                )
            )
        )
        self.assertIsNotNone(
            self.inspect(
                canonical.replace(
                    'subprocess.run(git_command(["status", "--short"]), env=git_environment)',
                    'git_environment |= {"GIT_CONFIG_PARAMETERS": "synthetic"}\n'
                    'subprocess.run(git_command(["status", "--short"]), env=git_environment)',
                )
            )
        )
        self.assertIsNotNone(
            self.inspect(
                canonical.replace(
                    'subprocess.run(git_command(["status", "--short"]), env=git_environment)',
                    'git_environment_alias = git_environment\n'
                    'git_environment_alias.update({"GIT_CONFIG_PARAMETERS": "synthetic"})\n'
                    'subprocess.run(git_command(["status", "--short"]), env=git_environment)',
                )
            )
        )
        self.assertIsNotNone(
            self.inspect(
                canonical.replace(
                    'subprocess.run(git_command(["status", "--short"]), env=git_environment)',
                    'git_environment["GIT_DIR"] = "/synthetic"\n'
                    'subprocess.run(git_command(["status", "--short"]), env=git_environment)',
                )
            )
        )
        self.assertIsNotNone(
            self.inspect(
                canonical.replace(
                    'subprocess.run(git_command(["status", "--short"]), env=git_environment)',
                    'def change_git_environment():\n'
                    '    git_environment.update({"GIT_CONFIG_PARAMETERS": "synthetic"})\n'
                    'change_git_environment()\n'
                    'subprocess.run(git_command(["status", "--short"]), env=git_environment)',
                )
            )
        )

    def test_python_git_builder_rebinding_is_not_certified(self) -> None:
        safe_environment = (
            '{"PATH": "/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM": "1", '
            '"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null", '
            '"GIT_ATTR_NOSYSTEM": "1"}'
        )
        for builder in ("git_command", "git_query"):
            prelude = (
                'import subprocess\n'
                'git_environment = {"PATH": "/usr/bin:/bin", '
                '"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", '
                '"GIT_CONFIG_SYSTEM": "/dev/null", "GIT_ATTR_NOSYSTEM": "1"}\n'
                f'def {builder}(arguments):\n'
                '    return ["/usr/bin/env", "-i", "GIT_CONFIG_NOSYSTEM=1", '
                '"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", '
                '"GIT_ATTR_NOSYSTEM=1", "/usr/bin/git", "--no-replace-objects", '
                '"-P", "-c", "core.fsmonitor=false", "-c", '
                '"core.hooksPath=/dev/null", *arguments]\n'
            )
            call = f'subprocess.run({builder}(["status", "--short"]), env=git_environment)\n'
            with self.subTest(builder=builder, kind="safe literal builder"):
                self.assertIsNone(self.inspect(prelude + call))
            with self.subTest(builder=builder, kind="rebound builder"):
                self.assertIsNotNone(self.inspect(
                    prelude + f'{builder} = lambda arguments: ["/usr/bin/git", "status"]\n' + call
                ))
            with self.subTest(builder=builder, kind="parameter shadow"):
                self.assertIsNotNone(self.inspect(
                    prelude
                    + f'def invoke({builder}):\n'
                    + f'    subprocess.run({builder}(["status", "--short"]), env={safe_environment})\n'
                    + 'invoke(lambda arguments: ["/usr/bin/git", "status"])\n'
                ))
            with self.subTest(builder=builder, kind="inner definition shadow"):
                self.assertIsNotNone(self.inspect(
                    prelude
                    + 'def invoke():\n'
                    + f'    def {builder}(arguments):\n'
                    + '        return ["/usr/bin/git", "status"]\n'
                    + f'    subprocess.run({builder}(["status", "--short"]), env={safe_environment})\n'
                    + 'invoke()\n'
                ))
            with self.subTest(builder=builder, kind="saved callable alias"):
                self.assertIsNotNone(self.inspect(
                    prelude
                    + f'{builder}_alias = {builder}\n'
                    + f'subprocess.run({builder}_alias(["status", "--short"]), env=git_environment)\n'
                ))

    def test_python_git_argv_mutation_is_not_certified(self) -> None:
        prelude = (
            'import subprocess\n'
            'git_environment = {"PATH": "/usr/bin:/bin", '
            '"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", '
            '"GIT_CONFIG_SYSTEM": "/dev/null", "GIT_ATTR_NOSYSTEM": "1"}\n'
            'def git_command(arguments):\n'
            '    return ["/usr/bin/env", "-i", "GIT_CONFIG_NOSYSTEM=1", '
            '"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", '
            '"GIT_ATTR_NOSYSTEM=1", "/usr/bin/git", "--no-replace-objects", '
            '"-P", "-c", "core.fsmonitor=false", "-c", '
            '"core.hooksPath=/dev/null", *arguments]\n'
            'argv = git_command(["status", "--short"])\n'
        )
        call = 'subprocess.run(argv, env=git_environment)\n'
        self.assertIsNone(self.inspect(prelude + call))
        for mutation in (
            'argv[:] = ["/usr/bin/git", "status"]\n',
            'saved_argv = argv\nsaved_argv[:] = ["/usr/bin/git", "status"]\n',
            'argv.append("--config-env=core.fsmonitor=GIT_CONFIG_PARAMETERS")\n',
            'def rewrite(command):\n'
            '    command[:] = ["/usr/bin/git", "status"]\n'
            'rewrite(argv)\n',
            'locals()["argv"][:] = ["/usr/bin/git", "status"]\n',
            'globals()["argv"][:] = ["/usr/bin/git", "status"]\n',
            'def rewrite():\n'
            '    global argv\n'
            '    argv[:] = ["/usr/bin/git", "status"]\n'
            'rewrite()\n',
        ):
            with self.subTest(mutation=mutation):
                self.assertIsNotNone(self.inspect(prelude + mutation + call))
        nonlocal_mutation = (
            'def wrapper():\n'
            '    argv = git_command(["status", "--short"])\n'
            '    def rewrite():\n'
            '        nonlocal argv\n'
            '        argv[:] = ["/usr/bin/git", "status"]\n'
            '    rewrite()\n'
            '    subprocess.run(argv, env=git_environment)\n'
            'wrapper()\n'
        )
        self.assertIsNotNone(self.inspect(prelude + nonlocal_mutation))
        chained_alias = (
            prelude.replace(
                'argv = git_command(["status", "--short"])\n',
                'argv = saved_argv = git_command(["status", "--short"])\n',
            )
            + 'saved_argv[:] = ["/usr/bin/git", "status"]\n'
            + call
        )
        self.assertIsNotNone(self.inspect(chained_alias))

    def test_warning_sink_and_absolute_path_do_not_disclose_local_data(self) -> None:
        self.assertIsNotNone(self.inspect(
            'import os, warnings\nwarnings.showwarning(os.environ, UserWarning, "x", 1)\n'
        ))
        self.assertIsNotNone(self.inspect(
            'from pathlib import Path\nprint(Path(".").absolute())\n'
        ))

    def test_module_values_cannot_escape_through_containers_or_helpers(self) -> None:
        unsafe = (
            'import os\nmods = [os]\nprint(mods[0].environ)\n',
            'import os\nmods = {"module": os}\nmods["module"].system("synthetic-command")\n',
            'import shutil\nmods = [shutil]\nmods[0].rmtree("synthetic-owned")\n',
            'import os\ndef module():\n    return os\nprint(module().environ)\n',
            'import os\ndef identity(value):\n    return value\nprint(identity(os).environ)\n',
            'import os\nmods = iter([os])\nprint(next(mods).environ)\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect('import os\nprint("reviewed")\n'))
        self.assertIsNone(self.inspect('import os\nalias = os\nprint("reviewed")\n'))

    def test_private_standard_library_reexports_are_refused(self) -> None:
        unsafe = (
            'import tempfile\ntempfile._shutil.rmtree("synthetic-owned")\n',
            'import tempfile\nprivate_tools = tempfile._shutil\n'
            'private_tools.rmtree("synthetic-owned")\n',
            'from tempfile import _shutil as private_tools\n'
            'private_tools.rmtree("synthetic-owned")\n',
            'import tempfile\n'
            'private_tools = getattr(tempfile, "_shutil")\n'
            'private_tools.rmtree("synthetic-owned")\n',
            'import tempfile\n'
            'module_items = vars(tempfile)\n'
            'module_items["_shutil"].rmtree("synthetic-owned")\n',
            'import tempfile\n'
            'module_items = tempfile.__dict__\n'
            'module_items["_shutil"].rmtree("synthetic-owned")\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect(
            'import tempfile\n'
            'with tempfile.TemporaryDirectory() as temporary:\n'
            '    print("reviewed")\n'
        ))
        self.assertIsNone(self.inspect(
            'from pathlib import Path\nPath("docs/backlog.json")\n'
        ))

    def test_warning_and_absolute_path_aliases_preserve_sensitive_values(self) -> None:
        unsafe = (
            'import os, warnings\nemit = warnings.showwarning\nemit(os.environ, UserWarning, "x", 1)\n',
            'import os\nimport warnings as w\nw.showwarning(os.environ, UserWarning, "x", 1)\n',
            'import os, warnings\nalias = warnings\nemit = alias.showwarning\nemit(os.environ, UserWarning, "x", 1)\n',
            'import os\nfrom warnings import showwarning as emit\nemit(os.environ, UserWarning, "x", 1)\n',
            'from pathlib import Path\nabsolute = Path(".").absolute\nprint(absolute())\n',
            'from pathlib import Path\nabsolute = getattr(Path("."), "absolute")\nprint(absolute())\n',
            'import os\nalias = os\nprint(alias.environb)\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

    def test_module_namespace_storage_requires_reviewed_uses(self) -> None:
        self.assertIsNone(self.inspect(
            'import os\nnamespace = {"module": os, "safe": "reviewed"}\n'
            'value = namespace["safe"]\nnamespace["safe"] = value\n'
        ))
        for suffix in (
            'print(namespace["module"].environ)\n',
            'key = "module"\nprint(namespace[key].environ)\n',
            'alias = namespace\nprint(alias["module"].environ)\n',
            'print(namespace.get("module").environ)\n',
            'print(list(namespace.values())[0].environ)\n',
            'namespace["safe"] = os\nprint(namespace["safe"].environ)\n',
        ):
            body = 'import os\nnamespace = {"module": os, "safe": "reviewed"}\n' + suffix
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

    def test_module_reflection_and_os_path_outputs_are_rejected(self) -> None:
        for body in (
            'import os, warnings\nemit = warnings.__dict__["showwarning"]\nemit(os.environ, UserWarning, "x", 1)\n',
            'import os\nprint(os.path.abspath("."))\n',
            'import os\nprint(os.path.realpath("."))\n',
            'import os\np = os.path\nprint(p.abspath("."))\n',
            'from os import path as p\nprint(p.abspath("."))\n',
            'from os import path as p\nmodules = [p]\nprint(modules[0].abspath("."))\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

    def test_os_path_callable_recovery_cannot_hide_local_output(self) -> None:
        for body in (
            'import os\nabsolute = os.path.abspath\nprint(absolute("."))\n',
            'import os\nrealpath = os.path.realpath\nprint(realpath("."))\n',
            'import os as platform\nprint(platform.path.abspath("."))\n',
            'import os\nplatform = os\nprint(platform.path.realpath("."))\n',
            'import os\nplatform = os\nabsolute = platform.path.abspath\nprint(absolute("."))\n',
            'import os\nfunctions = [os.path.abspath]\nprint(functions[0]("."))\n',
            'import os\nabsolute = getattr(os.path, "abspath")\nprint(absolute("."))\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect(
            'import os\nnormalized = os.path.abspath(".")\n'
            'assert normalized == os.path.realpath(".")\nprint("reviewed")\n'
        ))

    def test_pattern_bindings_cannot_shadow_reviewed_namespace_builtins(self) -> None:
        prefix = (
            'import ast, os\nnamespace = {"module": os}\n'
            'def fake(code, globals_dict):\n'
            '    print(globals_dict["module"].environ)\n'
        )
        call = (
            'exec(compile(ast.Module(body=[functions["safe"]], type_ignores=[]), '
            '"<probe>", "exec"), namespace)\n'
        )
        for pattern in ('case exec:', 'case [*exec]:', 'case {**exec}:'):
            with self.subTest(pattern=pattern):
                self.assertIsNotNone(self.inspect(
                    prefix + 'match fake:\n    ' + pattern + '\n        ' + call
                ))

    def test_os_path_member_recovery_and_expansion_are_not_certified(self) -> None:
        for body in (
            'from os import path as p\nalias = p\nprint(alias.abspath("."))\n',
            'from os import path as p\nalias = p\nprint(alias.realpath("."))\n',
            'import os\nresolve = os.path.__dict__["abspath"]\nprint(resolve("."))\n',
            'import os\nprint(os.path.expandvars("$GH_TOKEN"))\n',
            'import os\nexpand = os.path.expanduser\nprint(expand("~"))\n',
            'import os\nprint(os.path.os.environ)\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect(
            'import os\nvalue = os.path.normpath("docs/example")\n'
            'assert value == "docs/example"\nprint("reviewed")\n'
        ))

    def test_unreviewed_ast_code_cannot_use_reserved_compile_names(self) -> None:
        for body in (
            'import ast, os\n'
            'functions = {"safe": ast.parse("print(os.environ)").body[0]}\n'
            'namespace = {"os": os}\n'
            'exec(compile(ast.Module(body=[functions["safe"]], type_ignores=[]), '
            '"<probe>", "exec"), namespace)\n',
            'import ast, os\nmodule = ast.parse("print(os.environ)")\n'
            'namespace = {"os": os}\n'
            'exec(compile(module, "<probe>", "exec"), namespace)\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

    def packet_ast_source_prefix(self) -> str:
        return (
            'import ast, os\nfrom pathlib import Path\n'
            'packet = Path("docs/evidence/g01-recovery-packet.md").read_text()\n'
            'wrapper_start = packet.index("\\nimport hashlib\\n") + 1\n'
            'wrapper_end = packet.index("\\nPY\\n}", wrapper_start)\n'
            'wrapper = packet[wrapper_start:wrapper_end]\n'
            'module = ast.parse(wrapper)\n'
        )

    def test_packet_derived_ast_helper_selection_remains_supported(self) -> None:
        suffixes = (
            'functions = {node.name: node for node in module.body '
            'if isinstance(node, ast.FunctionDef)}\n'
            'namespace = {"os": os}\n'
            'exec(compile(ast.Module(body=[functions["run_go_child"]], type_ignores=[]), '
            '"<probe>", "exec"), namespace)\n',
            'selected = []\nfor node in module.body:\n'
            '    if isinstance(node, ast.FunctionDef) and node.name == "run_go_child":\n'
            '        selected.append(node)\n'
            'namespace = {"os": os}\n'
            'exec(compile(ast.Module(body=selected, type_ignores=[]), '
            '"<probe>", "exec"), namespace)\n',
        )
        for suffix in suffixes:
            with self.subTest(suffix=suffix):
                self.assertIsNone(self.inspect(self.packet_ast_source_prefix() + suffix))

    def test_ast_compile_provenance_is_invalidated_by_unreviewed_mutation(self) -> None:
        suffixes = (
            'functions = {node.name: node for node in module.body '
            'if isinstance(node, ast.FunctionDef)}\n'
            'functions["run_go_child"] = ast.parse("print(os.environ)").body[0]\n'
            'namespace = {"os": os}\n'
            'exec(compile(ast.Module(body=[functions["run_go_child"]], type_ignores=[]), '
            '"<probe>", "exec"), namespace)\n',
            'selected = []\nfor node in module.body:\n    selected.append(node)\n'
            'selected.append(ast.parse("print(os.environ)").body[0])\n'
            'namespace = {"os": os}\n'
            'exec(compile(ast.Module(body=selected, type_ignores=[]), '
            '"<probe>", "exec"), namespace)\n',
            'changed = wrapper\nchanged = "print(os.environ)"\n'
            'module = ast.parse(changed)\nnamespace = {"os": os}\n'
            'exec(compile(module, "<probe>", "exec"), namespace)\n',
        )
        for suffix in suffixes:
            with self.subTest(suffix=suffix):
                self.assertIsNotNone(self.inspect(self.packet_ast_source_prefix() + suffix))

    def test_compile_provenance_does_not_leak_across_parameters_or_opaque_calls(self) -> None:
        specimens = (
            self.packet_ast_source_prefix() +
            'def execute(wrapper):\n    exec(compile(wrapper, "<probe>", "exec"), {"os": os})\n'
            'execute("print(os.environ)")\n',
            self.packet_ast_source_prefix() +
            'def execute(helper_source):\n    exec(compile(helper_source, "<probe>", "exec"), {"os": os})\n'
            'callback = execute\nexecute(wrapper)\ncallback("print(os.environ)")\n',
            self.packet_ast_source_prefix() +
            'def alter(value):\n    value.body = ast.parse("print(os.environ)").body\n'
            'alter(module)\nexec(compile(module, "<probe>", "exec"), {"os": os})\n',
        )
        for specimen in specimens:
            with self.subTest(specimen=specimen):
                self.assertIsNotNone(self.inspect(specimen))

    def test_source_replacement_cannot_inject_an_unreviewed_program(self) -> None:
        specimen = self.packet_ast_source_prefix() + (
            'wrapper = wrapper.replace("", "print(os.environ)\\n", 1)\n'
            'exec(compile(wrapper, "<probe>", "exec"), {"os": os})\n'
        )
        self.assertIsNotNone(self.inspect(specimen))

    def test_ast_parameters_and_readonly_call_spellings_cannot_inherit_provenance(self) -> None:
        specimens = (
            'def execute(module):\n'
            '    exec(compile(ast.Module(body=[module], type_ignores=[]), "<probe>", "exec"), {"os": os})\n'
            'execute(ast.parse("print(os.environ)").body[0])\n',
            'def alter(value):\n    value.body = ast.parse("print(os.environ)").body\n    return True\n'
            'functions = {node.name: node for node in module.body if alter(node)}\n'
            'exec(compile(ast.Module(body=[functions["run_go_child"]], type_ignores=[]), "<probe>", "exec"), {"os": os})\n',
            'def len(value):\n    value.body = ast.parse("print(os.environ)").body\n    return 1\n'
            'len(module)\nexec(compile(module, "<probe>", "exec"), {"os": os})\n',
        )
        for specimen in specimens:
            with self.subTest(specimen=specimen):
                self.assertIsNotNone(self.inspect(self.packet_ast_source_prefix() + specimen))

    def test_augmented_assignment_and_forged_source_providers_invalidate_compile_proof(self) -> None:
        prefix = self.packet_ast_source_prefix()
        specimens = (
            prefix + 'wrapper += "\\nprint(os.environ)"\nexec(compile(wrapper, "<probe>", "exec"), {"os": os})\n',
            prefix + 'module.body += ast.parse("print(os.environ)").body\nexec(compile(module, "<probe>", "exec"), {"os": os})\n',
            prefix + 'def execute(helper_source):\n    helper_source += "\\nprint(os.environ)"\n    exec(compile(helper_source, "<probe>", "exec"), {"os": os})\nexecute(wrapper)\n',
            'import os\nfrom pathlib import Path\npacket_path = "docs/evidence/g01-recovery-packet.md"\n'
            'for packet_path in ("outside",):\n    pass\nwrapper = Path(packet_path).read_text()\n'
            'exec(compile(wrapper, "<probe>", "exec"), {"os": os})\n',
            'import os, subprocess\nwrapper = subprocess.check_output(["git", "--no-replace-objects", "show", "'
            + 'f' * 40 + ':docs/evidence/g01-recovery-packet.md"], text=True)\n'
            'exec(compile(wrapper, "<probe>", "exec"), {"os": os})\n',
            'import os\nfrom pathlib import Path\ndef fake(self):\n    return "print(os.environ)"\n'
            'setattr(Path, "read_text", fake)\nwrapper = Path("docs/evidence/g01-recovery-packet.md").read_text()\n'
            'exec(compile(wrapper, "<probe>", "exec"), {"os": os})\n',
        )
        for specimen in specimens:
            with self.subTest(specimen=specimen):
                self.assertIsNotNone(self.inspect(specimen))

    def test_reused_comprehension_targets_and_wrapped_ast_arguments_keep_mutation_taint(self) -> None:
        setup = self.packet_ast_source_prefix() + (
            'def alter(value):\n    value.body = ast.parse("print(os.environ)").body\n'
            'def alter_wrapped(values):\n    alter(values[0])\n'
        )
        for mutation in (
            '[alter(node) for node in ["unused"] for node in [module]]\n',
            'alter_wrapped([module])\n',
        ):
            with self.subTest(mutation=mutation):
                self.assertIsNotNone(self.inspect(setup + mutation +
                    'exec(compile(module, "<probe>", "exec"), {"os": os})\n'))

    def test_ast_child_reader_results_cannot_hide_mutation_from_the_compiled_origin(self) -> None:
        setup = self.packet_ast_source_prefix() + (
            'functions = {node.name: node for node in module.body if isinstance(node, ast.FunctionDef)}\n'
        )
        specimens = (
            'helper = functions.get("run_go_child")\nhelper.body = ast.parse("print(os.environ)").body\n'
            'exec(compile(ast.Module(body=[functions["run_go_child"]], type_ignores=[]), "<probe>", "exec"), {"os": os})\n',
            'selected = next(ast.walk(module))\nselected.body = ast.parse("print(os.environ)").body\n'
            'exec(compile(module, "<probe>", "exec"), {"os": os})\n',
            'arguments = functions["run_go_child"].args\n'
            'arguments.defaults = [ast.parse("print(os.environ)").body[0].value]\n'
            'exec(compile(ast.Module(body=[functions["run_go_child"]], type_ignores=[]), "<probe>", "exec"), {"os": os})\n',
        )
        for specimen in specimens:
            with self.subTest(specimen=specimen):
                self.assertIsNotNone(self.inspect(setup + specimen))

    def test_deferred_git_provider_does_not_ignore_later_monkeypatch(self) -> None:
        specimen = (
            'import os, subprocess\n'
            'def run_probe():\n'
            '    wrapper = subprocess.check_output(["git", "--no-replace-objects", "show", '
            '"01764bbed0a387129d2a2abbc9e27a87e073f87e:docs/evidence/g01-recovery-packet.md"], text=True)\n'
            '    exec(compile(wrapper, "<probe>", "exec"), {"os": os})\n'
            'def fake(*args, **kwargs):\n    return "print(os.environ)"\n'
            'subprocess.check_output = fake\nrun_probe()\n'
        )
        self.assertIsNotNone(self.inspect(specimen))

    def test_opaque_ast_receiver_methods_cannot_replace_compiled_children(self) -> None:
        suffix = 'exec(compile(module, "<probe>", "exec"), {"os": os})\n'
        for mutation in (
            'module.body.__iadd__(ast.parse("print(os.environ)").body)\n',
            'module.body.__init__(ast.parse("print(os.environ)").body)\n',
            'reset = module.body.__init__\nreset(ast.parse("print(os.environ)").body)\n',
        ):
            with self.subTest(mutation=mutation):
                self.assertIsNotNone(self.inspect(self.packet_ast_source_prefix() + mutation + suffix))

    def test_path_provider_patch_aliases_do_not_retain_source_authority(self) -> None:
        setup = 'import os\nfrom pathlib import Path\ndef fake(self):\n    return "print(os.environ)"\n'
        suffix = (
            'wrapper = Path("docs/evidence/g01-recovery-packet.md").read_text()\n'
            'exec(compile(wrapper, "<probe>", "exec"), {"os": os})\n'
        )
        for mutation in (
            'patch_reader = setattr\npatch_reader(Path, "read_text", fake)\n',
            'PathAlias = Path\nPathAlias.read_text = fake\n',
            'patches = {"set": setattr}\npatches["set"](Path, "read_text", fake)\n',
            'def alter(cls):\n    cls.read_text = fake\nalter(Path)\n',
            'classes = {"reader": Path}\nclasses["reader"].read_text = fake\n',
            'PathAlias, = [Path]\nPathAlias.read_text = fake\n',
            'PathAlias, = {Path}\nPathAlias.read_text = fake\n',
            'classes = {"reader": Path}\nPathAlias = classes.get("reader")\nPathAlias.read_text = fake\n',
            'classes = [Path]\nPathAlias = classes.pop()\nPathAlias.read_text = fake\n',
            'import pathlib as p\nPathAlias = p.Path\nPathAlias.read_text = fake\n',
            'import pathlib\nPathAlias = getattr(pathlib, "Path")\nPathAlias.read_text = fake\n',
            'def reader_class():\n    return Path\nPathAlias = reader_class()\nPathAlias.read_text = fake\n',
            'classes = {"reader": Path}\npick = classes.get\nPathAlias = pick("reader")\nPathAlias.read_text = fake\n',
            'classes = [Path]\npick = classes.pop\nPathAlias = pick()\nPathAlias.read_text = fake\n',
            'PathAlias = next(cls for cls in [Path])\nPathAlias.read_text = fake\n',
            'classes = [cls for cls in [Path]]\nPathAlias = classes.pop()\nPathAlias.read_text = fake\n',
            'classes = [Path]\nwalk = classes.__iter__\nPathAlias = next(walk())\nPathAlias.read_text = fake\n',
            'classes = {"reader": Path}\npick = classes.get\nagain = pick\npick = again\nPathAlias = pick("reader")\nPathAlias.read_text = fake\n',
        ):
            with self.subTest(mutation=mutation):
                self.assertIsNotNone(self.inspect(setup + mutation + suffix))

    def test_path_provider_comprehension_result_origins_invalidate_authority(self) -> None:
        setup = 'import os\nfrom pathlib import Path\ndef fake(self):\n    return "print(os.environ)"\n'
        suffix = (
            'PathAlias.read_text = fake\n'
            'wrapper = Path("docs/evidence/g01-recovery-packet.md").read_text()\n'
            'exec(compile(wrapper, "<probe>", "exec"), {"os": os})\n'
        )
        for borrow in (
            'PathAlias = next(Path for _ in [0])\n',
            'classes = [Path for _ in [0]]\nPathAlias = classes.pop()\n',
            'classes = {Path for _ in [0]}\nPathAlias = classes.pop()\n',
            'classes = {"reader": Path for _ in [0]}\nPathAlias = classes.get("reader")\n',
            'classes = {Path: "reader" for _ in [0]}\nPathAlias, _ = classes.popitem()\n',
            'PathAlias = next((Path if flag else None) for flag in [True])\n',
        ):
            with self.subTest(borrow=borrow):
                self.assertIsNotNone(self.inspect(setup + borrow + suffix))
        for control in (
            'PathAlias = next(Path("reviewed") for _ in [0])\n',
            'classes = [Path("reviewed") for _ in [0]]\nPathAlias = classes.pop()\n',
            'classes = {"reader": 0 for _ in [0]}\nPathAlias = classes.get("reader")\n',
        ):
            with self.subTest(control=control):
                tree = ast.parse(setup + control + 'PathAlias.read_text = fake\n')
                self.assertFalse(self.scanner["python_compile_primitive_is_shadowed"]("Path", tree))

    def test_boolean_path_provider_results_cannot_patch_trusted_reader(self) -> None:
        setup = 'import os\nfrom pathlib import Path\ndef fake(self):\n    return "print(os.environ)"\n'
        for borrow in (
            'PathAlias = next((Path or None) for _ in [0])\n',
            'PathAlias = True and Path\n',
        ):
            with self.subTest(borrow=borrow):
                self.assertIsNotNone(self.inspect(setup + borrow +
                    'PathAlias.read_text = fake\n'
                    'wrapper = Path("docs/evidence/g01-recovery-packet.md").read_text()\n'
                    'exec(compile(wrapper, "<probe>", "exec"), {"os": os})\n'))

    def test_historical_compile_sources_require_replacement_suppression(self) -> None:
        reference = '01764bbed0a387129d2a2abbc9e27a87e073f87e:docs/evidence/g01-recovery-packet.md'
        common = (
            'import os, subprocess\n'
            'git_environment = {"PATH": "/usr/bin:/bin", '
            '"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", '
            '"GIT_CONFIG_SYSTEM": "/dev/null", "GIT_ATTR_NOSYSTEM": "1"}\n'
        )
        prefix = (
            '["/usr/bin/env", "-i", "GIT_CONFIG_NOSYSTEM=1", '
            '"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", '
            '"GIT_ATTR_NOSYSTEM=1", "/usr/bin/git", '
        )
        suffix = (
            '"-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", '
            '"show", ' + repr(reference) + '], env=git_environment, text=True)\n'
            'exec(compile(wrapper, "<probe>", "exec"), {"os": os})\n'
        )
        safe_argv_prefix = prefix + '"--no-replace-objects", "-P", '
        unsafe_argvs = (
            (
                "missing replacement suppression",
                prefix + '"-P", ' + suffix,
            ),
            (
                "missing system config",
                (safe_argv_prefix + suffix).replace(
                    '"GIT_CONFIG_SYSTEM=/dev/null", ', "", 1
                ),
            ),
            (
                "missing fsmonitor fence",
                (safe_argv_prefix + suffix).replace(
                    '"-c", "core.fsmonitor=false", ', "", 1
                ),
            ),
            (
                "missing hooks fence",
                (safe_argv_prefix + suffix).replace(
                    '"-c", "core.hooksPath=/dev/null", ', "", 1
                ),
            ),
        )
        for label, argv in unsafe_argvs:
            body = common + 'wrapper = subprocess.check_output(' + argv
            with self.subTest(unsafe_control=label):
                self.assertIsNotNone(self.inspect(body))
        safe_options = '"--no-replace-objects", "-P", '
        direct = common + 'wrapper = subprocess.check_output(' + prefix + safe_options + suffix
        with self.subTest(safe_prefix="canonical isolated direct argv"):
            self.assertIsNone(self.inspect(direct))
        tuple_argv = direct.replace(
            'subprocess.check_output([', 'subprocess.check_output((', 1
        ).replace(
            '"show", ' + repr(reference) + '], env=git_environment',
            '"show", ' + repr(reference) + ') , env=git_environment',
            1,
        )
        with self.subTest(safe_prefix="canonical isolated tuple argv"):
            self.assertIsNone(self.inspect(tuple_argv))

    def test_arbitrary_packet_slices_do_not_acquire_compile_authority(self) -> None:
        setup = 'import os\nfrom pathlib import Path\npacket = Path("docs/evidence/g01-recovery-packet.md").read_text()\n'
        for extraction in (
            'needle = "print(os.environ)"\nstart = packet.index(needle)\nwrapper = packet[start:start + len(needle)]\n',
            'start = packet.index("print(os.environ)")\nend = start + 17\nwrapper = packet[start:end]\n',
            'start = packet.index("print(os.environ)")\nend = start + 17\n',
        ):
            with self.subTest(extraction=extraction):
                argument = 'packet[start:end]' if 'wrapper =' not in extraction else 'wrapper'
                self.assertIsNotNone(self.inspect(setup + extraction +
                    'exec(compile(' + argument + ', "<probe>", "exec"), {"os": os})\n'))

    def test_packet_slice_bounds_do_not_fall_back_across_parameter_shadows(self) -> None:
        specimen = (
            'import os\nfrom pathlib import Path\n'
            'packet = Path("docs/evidence/g01-recovery-packet.md").read_text()\n'
            'start = packet.index("\\nimport hashlib\\n", packet.index("go_test_checked()")) + 1\n'
            'end = packet.index("\\nPY\\n}", start)\n'
            'def execute(start, end):\n'
            '    exec(compile(packet[start:end], "<probe>", "exec"), {"os": os})\n'
            'needle = "print(os.environ)"\nbad_start = packet.index(needle)\n'
            'execute(bad_start, bad_start + len(needle))\n'
        )
        self.assertIsNotNone(self.inspect(specimen))

    def test_direct_packet_slice_retains_existing_compile_base_allowlist(self) -> None:
        specimen = (
            'import os\nfrom pathlib import Path\n'
            'unlisted_source = Path("docs/evidence/g01-recovery-packet.md").read_text()\n'
            'start = unlisted_source.index("\\nimport hashlib\\n", unlisted_source.index("go_test_checked()")) + 1\n'
            'end = unlisted_source.index("\\nPY\\n}", start)\n'
            'exec(compile(unlisted_source[start:end], "<probe>", "exec"), {"os": os})\n'
        )
        self.assertIsNotNone(self.inspect(specimen))

    def test_same_line_packet_bound_reassignments_invalidate_slice_recipe(self) -> None:
        specimen = (
            'import os\nfrom pathlib import Path\n'
            'packet = Path("docs/evidence/g01-recovery-packet.md").read_text()\n'
            'start = packet.index("\\nimport hashlib\\n") + 1; start = packet.index("print(os.environ)")\n'
            'end = packet.index("\\nPY\\n}", start); end = packet.index("print(os.environ)") + 17\n'
            'exec(compile(packet[start:end], "<probe>", "exec"), {"os": os})\n'
        )
        self.assertIsNotNone(self.inspect(specimen))

    def test_actual_packet_compile_helpers_retain_provenance(self) -> None:
        checked = 0
        for number, body, safe_marker, _ in self.scanner["python_heredoc_bodies"](PACKET_TEXT):
            tree = ast.parse(body, filename="<packet-helper-data>")
            for node in ast.walk(tree):
                if not (
                    isinstance(node, ast.Call) and isinstance(node.func, ast.Name)
                    and node.func.id == "exec" and node.args
                    and isinstance(node.args[0], ast.Call)
                    and isinstance(node.args[0].func, ast.Name)
                    and node.args[0].func.id == "compile"
                ):
                    continue
                checked += 1
                with self.subTest(packet_line=number, body_line=node.lineno):
                    self.assertTrue(self.scanner["reviewed_python_exec_call"](node, safe_marker, tree))
        self.assertGreater(checked, 0)
        print(f"packet compile-source boundary: {checked} actual helper calls checked")

    def test_historic_scanner_loaders_export_only_their_required_helpers(self) -> None:
        checked = 0
        for number, body, safe_marker, _ in self.scanner["python_heredoc_bodies"](PACKET_TEXT):
            tree = ast.parse(body, filename="<historic-loader-data>")
            for function in ast.walk(tree):
                if not isinstance(function, ast.FunctionDef) or function.name != "load_scanner":
                    continue
                checked += 1
                with self.subTest(packet_line=number):
                    exported = [node.value for node in function.body if isinstance(node, ast.Return)]
                    self.assertEqual(1, len(exported))
                    self.assertIsInstance(exported[0], ast.Dict)
                    exports = exported[0]
                    keys = {key.value for key in exports.keys if isinstance(key, ast.Constant)}
                    required = {
                        node.slice.value for node in ast.walk(tree)
                        if isinstance(node, ast.Subscript) and isinstance(node.slice, ast.Constant)
                        and isinstance(node.slice.value, str) and node.slice.value in self.scanner
                        and not (isinstance(node.value, ast.Name) and node.value.id == "loader_namespace")
                    }
                    self.assertEqual(required, keys)
                    self.assertTrue(all(
                        isinstance(value, ast.Subscript) and isinstance(value.value, ast.Name)
                        and value.value.id == "loader_namespace" and isinstance(value.slice, ast.Constant)
                        and value.slice.value == key.value
                        for key, value in zip(exports.keys, exports.values)
                    ))
                    self.assertIsNone(self.scanner["inspect_python_heredoc"](body, safe_marker))
        self.assertEqual(10, checked)

    def test_ast_alias_and_nested_mutations_invalidate_helper_origins(self) -> None:
        mutations = (
            'alias = functions\nalias["run_go_child"] = ast.parse("print(os.environ)").body[0]\n',
            'helper = functions["run_go_child"]\nhelper.body = ast.parse("print(os.environ)").body\n',
            'functions["run_go_child"].body = ast.parse("print(os.environ)").body\n',
            'put = functions.update\nput({"run_go_child": ast.parse("print(os.environ)").body[0]})\n',
            'helper = functions["run_go_child"]\nsetattr(helper, "body", ast.parse("print(os.environ)").body)\n',
        )
        setup = self.packet_ast_source_prefix() + (
            'functions = {node.name: node for node in module.body '
            'if isinstance(node, ast.FunctionDef)}\n'
        )
        suffix = (
            'namespace = {"os": os}\n'
            'exec(compile(ast.Module(body=[functions["run_go_child"]], type_ignores=[]), '
            '"<probe>", "exec"), namespace)\n'
        )
        for mutation in mutations:
            with self.subTest(mutation=mutation):
                self.assertIsNotNone(self.inspect(setup + mutation + suffix))

    def test_ast_providers_cannot_be_forged_by_markers_or_monkeypatch(self) -> None:
        bodies = (
            'import ast, os, subprocess\n'
            'packet = subprocess.check_output(["printf", '
            '"print(os.environ) # docs/evidence/g01-recovery-packet.md"], text=True)\n'
            'module = ast.parse(packet)\nnamespace = {"os": os}\n'
            'exec(compile(module, "<probe>", "exec"), namespace)\n',
            self.packet_ast_source_prefix() +
            'def fake(value):\n    return None\nast.parse = fake\n'
            'module = ast.parse(wrapper)\nnamespace = {"os": os}\n'
            'exec(compile(module, "<probe>", "exec"), namespace)\n',
        )
        for body in bodies:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

    def test_compile_primitives_and_source_parameters_need_proven_bindings(self) -> None:
        bodies = (
            self.packet_ast_source_prefix() +
            'exec = eval\nnamespace = {}\n'
            'exec(compile(module, "<probe>", "exec"), namespace)\n',
            'def check(helper_source):\n'
            '    marker = \'source = Path("docs/evidence/g01-recovery-packet.md")\'\n'
            '    tail = "matches = []"\n'
            '    exec(compile(helper_source, "<probe>", "exec"), {})\n'
            'check("print(__import__(\\\"os\\\").environ)")\n',
        )
        for body in bodies:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

    def test_bash_prompt_expansion_cannot_evaluate_credential_name(self) -> None:
        self.assertIsNotNone(self.shell_document_violation(
            "printf -v payload '%s%s' '$' 'GH_TOKEN'; printf '%s\\n' \"${payload@P}\""
        ))
        self.assertIsNone(self.shell_violation("printf '%s\\n' 'reviewed'"))

    def test_shell_reader_rejects_unreviewed_relative_credential_file(self) -> None:
        self.assertIsNotNone(self.shell_violation("awk '{print}' maintainer.pem"))
        self.assertIsNone(self.shell_violation("awk '{print}' docs/EXECUTION.md"))

    def test_exception_arguments_keep_environment_taint(self) -> None:
        self.assertIsNotNone(self.inspect('import os\nraise RuntimeError(os.environ)\n'))
        self.assertIsNotNone(self.inspect(
            'import os\nraise RuntimeError("reviewed") from RuntimeError(os.environ)\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import os\ncause = RuntimeError(os.environ)\n'
            'raise RuntimeError("reviewed") from cause\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import os\ncause = RuntimeError(RuntimeError(os.environ))\n'
            'raise RuntimeError("reviewed") from cause\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import os\ninner = RuntimeError(os.environ)\n'
            'cause = RuntimeError(inner)\nraise RuntimeError("reviewed") from cause\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import os\ninner = StopIteration(os.environ)\n'
            'cause = RuntimeError(inner)\nraise RuntimeError("reviewed") from cause\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import os\ninner = UserWarning(os.environ)\n'
            'cause = RuntimeError(inner)\nraise RuntimeError("reviewed") from cause\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import os\nclass Halt(Exception):\n    pass\n'
            'inner = Halt(os.environ)\ncause = RuntimeError(inner)\n'
            'raise RuntimeError("reviewed") from cause\n'
        ))
        self.assertIsNone(self.inspect('raise RuntimeError("reviewed")\n'))

    def test_assertion_message_cannot_disclose_environment(self) -> None:
        self.assertIsNotNone(self.inspect('import os\nassert False, os.environ\n'))
        self.assertIsNone(self.inspect('assert True, "reviewed"\n'))

    def test_module_dictionary_environment_access_is_rejected(self) -> None:
        for body in (
            'import os\nprint(vars(os)["environ"])\n',
            'import os\nprint(os.__dict__["environ"])\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect('import os\nprint("reviewed")\n'))

    def test_getattr_alias_cannot_expose_os_environment(self) -> None:
        self.assertIsNotNone(self.inspect(
            'import os\nlookup = getattr\nprint(lookup(os, "environ"))\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import os\nlookup = getattr\nprint(lookup([os][0], "environ"))\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\nlookup = getattr\nprint("reviewed")\n'
        ))

    def test_assigned_sys_alias_cannot_reach_frame_namespace(self) -> None:
        self.assertIsNotNone(self.inspect(
            'import sys\nalias = sys\nalias._getframe()\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import sys\nalias, = (sys,)\nalias._getframe()\n'
        ))
        self.assertIsNone(self.inspect('import sys\nalias = sys\nprint("reviewed")\n'))

    def test_shutil_module_assignment_alias_cannot_hide_mutation(self) -> None:
        self.assertIsNotNone(self.inspect(
            'import shutil\nalias = shutil\nalias.rmtree("/tmp/maintainer-owned")\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import shutil\nalias, = (shutil,)\nalias.rmtree("/tmp/maintainer-owned")\n'
        ))
        self.assertIsNone(self.inspect('import shutil\nalias = shutil\nprint("reviewed")\n'))

    def test_unreviewed_shutil_entry_point_is_rejected(self) -> None:
        self.assertIsNotNone(self.inspect(
            'import shutil\nshutil._rmtree_unsafe("/tmp/maintainer-owned", None, lambda *args: None)\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import shutil\ngetattr(shutil, "_rmtree_unsafe")("/tmp/maintainer-owned", None, lambda *args: None)\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import shutil\nvars(shutil)["_rmtree_unsafe"]("/tmp/maintainer-owned", None, lambda *args: None)\n'
        ))
        self.assertIsNone(self.inspect('import shutil\nprint("reviewed")\n'))

    def test_signal_aliases_require_owned_targets(self) -> None:
        self.assertIsNotNone(self.inspect(
            'from os import getppid, kill\nkill(getppid(), 9)\n'
        ))
        self.assertIsNotNone(self.inspect(
            'import os\nsend = os.kill\nsend(1, 9)\n'
        ))
        self.assertIsNone(self.inspect('import os\nprint("reviewed")\n'))

    def test_from_imported_os_and_shutil_calls_use_fail_closed_classification(self) -> None:
        for body in (
            'from os import removedirs\nremovedirs("/tmp/maintainer-owned/empty/leaf")\n',
            'from os import renames\nrenames("/tmp/a", "/tmp/b")\n',
            'from os import removedirs as wipe\nwipe("/tmp/maintainer-owned/empty/leaf")\n',
            'from shutil import disk_usage\nprint(disk_usage("/"))\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect('from shutil import which\nprint(which("git"))\n'))

    def test_from_import_aliases_resolve_at_call_site(self) -> None:
        for body in (
            'from os import removedirs as action\n'
            'action("/tmp/maintainer-owned/a")\n'
            'from shutil import which as action\n',
            'flag = True\n'
            'from os import removedirs as action\n'
            'if flag:\n'
            '    from shutil import which as action\n'
            'action("/tmp/maintainer-owned/a")\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect(
            'from os import removedirs as action\n'
            'def f():\n'
            '    from shutil import which as action\n'
            '    action("git")\n'
        ))
        self.assertIsNone(self.inspect(
            'action("/tmp/maintainer-owned/a")\n'
            'from os import removedirs as action\n'
        ))

    def test_launchers_bound_through_callable_defaults_stay_classified(self) -> None:
        for body in (
            'import os\n(lambda launch=os.system: launch("rm -rf /tmp/maintainer-owned"))()\n',
            'import os\ndef f(launch=os.system):\n    launch("rm -rf /tmp/maintainer-owned")\nf()\n',
            'import subprocess\ndef f(run=subprocess.run):\n    run(["gh", "workflow", "run", "ci.yml"])\nf()\n',
            'import os\ndef f(*, launch=os.system):\n    launch("rm -rf /tmp/maintainer-owned")\nf()\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect('def f(launch=None):\n    print("reviewed")\nf()\n'))
        self.assertIsNone(self.inspect('def f(fn=len):\n    print(fn("reviewed"))\nf()\n'))
        self.assertIsNone(self.inspect('import os\ndef f(launch=os.system):\n    print("reviewed")\nf()\n'))

    def test_launchers_nested_in_default_expressions_stay_classified(self) -> None:
        for body in (
            'import os\nflag = True\ndef f(launch=os.system if flag else len):\n'
            '    launch("rm -rf /tmp/maintainer-owned")\nf()\n',
            'import os\ndef f(launch=(os.system, len)[0]):\n'
            '    launch("rm -rf /tmp/maintainer-owned")\nf()\n',
            'import os\ndef f(launch={"a": os.system}["a"]):\n'
            '    launch("rm -rf /tmp/maintainer-owned")\nf()\n',
            'import os\ndef f(launch=len and os.system):\n'
            '    launch("rm -rf /tmp/maintainer-owned")\nf()\n',
            'import os\ndef f(launch=(lambda: os.system)()):\n'
            '    launch("rm -rf /tmp/maintainer-owned")\nf()\n',
            'import os\ndef f(launch=(runner := os.system)):\n'
            '    launch("rm -rf /tmp/maintainer-owned")\nf()\n',
            'from os import system\ndef f(launch=system if True else len):\n'
            '    launch("rm -rf /tmp/maintainer-owned")\nf()\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect(
            'def f(cb=len if True else str):\n    print(cb("8"))\nf()\n'
        ))
        self.assertIsNone(self.inspect(
            'def run(cmd):\n    return cmd\ndef f(cb=run):\n    print(cb("8"))\nf()\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\ndef f(launch=os.system if True else len):\n'
            '    print("reviewed")\nf()\n'
        ))

    def test_python_heredoc_requires_quoted_delimiter(self) -> None:
        for opener in (
            "/opt/homebrew/bin/python3 -I - <<PY",
            "/opt/homebrew/bin/python3 -I - <<-PY",
        ):
            with self.subTest(opener=opener):
                segments = list(self.scanner["shell_token_segments"](opener))  # type: ignore[operator]
                self.assertTrue(
                    any(
                        self.scanner["python_stdin_command"](segment)  # type: ignore[operator]
                        for segment in segments
                    )
                )
                self.assertFalse(
                    any(
                        self.scanner["reviewed_python_heredoc_segment"](opener, segment)  # type: ignore[operator]
                        for segment in segments
                    )
                )
        for opener in (
            "/opt/homebrew/bin/python3 -I - <<'PY'",
            '/opt/homebrew/bin/python3 -I - <<"PY"',
            "/opt/homebrew/bin/python3 -I - <<-'PY'",
        ):
            with self.subTest(opener=opener):
                segments = list(self.scanner["shell_token_segments"](opener))  # type: ignore[operator]
                self.assertTrue(
                    any(
                        self.scanner["reviewed_python_heredoc_segment"](opener, segment)  # type: ignore[operator]
                        for segment in segments
                    )
                )
        quoted_fixture = "```sh\n/opt/homebrew/bin/python3 -I - <<'PY'\nprint(\"reviewed\")\nPY\n```\n"
        bodies = list(self.scanner["python_heredoc_bodies"](quoted_fixture))  # type: ignore[operator]
        self.assertEqual(1, len(bodies))
        self.assertTrue(bodies[0][3].get("quoted"))
        unquoted_fixture = "```sh\n/opt/homebrew/bin/python3 -I - <<PY\nprint(\"reviewed\")\nPY\n```\n"
        bodies = list(self.scanner["python_heredoc_bodies"](unquoted_fixture))  # type: ignore[operator]
        self.assertEqual(1, len(bodies))
        self.assertFalse(bodies[0][3].get("quoted"))

    def test_sensitive_operands_are_rejected_before_implicit_exception_messages(self) -> None:
        for body in (
            'import os\n{}[str(os.environ)]\n',
            'import os\nsecret = os.environ\n{}[secret]\n',
            'import os\nsecret = str(os.environ)\n[][secret]\n',
            'import os\nsecret = str(os.environ)\nint(secret)\n',
            'import os\nsecret = str(os.environ)\nfloat(secret)\n',
            'import os\nint(str(os.environ))\n',
            'import os\nfor value in os.environ.values():\n    {}[value]\n',
            'import os\nsecret = os.environ\nkey = str(secret).strip()\n{}[key]\n',
            'import os\nsecret = str(os.environ)\ndel {}[secret]\n',
            'import os\ndef k():\n    return str(os.environ)\n{}[k()]\n',
            'import os\nsecrets = [str(os.environ)]\nfor _, v in enumerate(secrets):\n    {}[v]\n',
            'import os\nfor key, value in os.environ.items():\n    {}[value]\n',
            'import os\nto_int = int\nsecret = str(os.environ)\nto_int(secret)\n',
            'import os\nto_float = float\nsecret = str(os.environ)\nto_float(secret)\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect('d = {"a": 1}\nprint(d["a"])\n'))
        self.assertIsNone(self.inspect('print(int("3"))\n'))
        self.assertIsNone(self.inspect('items = [1, 2]\nprint(items[0])\n'))
        self.assertIsNone(self.inspect('import sys\nprint(sys.argv[1:])\n'))
        self.assertIsNone(self.inspect(
            'import os\nsecret = str(os.environ)\nd = {}\nd[secret] = 1\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\ndefaults = {"PATH": "/x"}\n'
            'for key in os.environ:\n'
            '    print(defaults[key] if key in defaults else "none")\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\ndef other():\n    secret = os.environ\n    return 1\n'
            'def f():\n    secret = 0\n    d = {}\n    return d[secret]\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\nenv = dict(os.environ)\ndefaults = {}\n'
            'for key in env:\n    defaults[key] = 1\n'
            'total = sum(defaults[key] for key in env)\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\nfor key in os.environ.keys():\n    {}[key]\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\nfor key in dict(os.environ):\n    {}[key]\n'
        ))

    def test_complex_conversions_are_rejected_before_implicit_exception_messages(self) -> None:
        for body in (
            'import os\ncomplex(str(os.environ))\n',
            'import os\nto_complex = complex\nsecret = str(os.environ)\nto_complex(secret)\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect('print(complex(0, 1))\n'))
        self.assertIsNone(self.inspect('print(complex("3"))\n'))

    def test_conversion_callables_in_defaults_and_containers_are_tracked(self) -> None:
        for body in (
            'import os\ndef f(convert=int):\n    convert(str(os.environ))\nf()\n',
            'import os\nconverters = [float]\nconverters[0](str(os.environ))\n',
            'import os\nto_int = int\ndef f(convert=to_int):\n    convert(str(os.environ))\nf()\n',
            'import os\ndef f(convert):\n    convert(str(os.environ))\nf(int)\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect(
            'import os\ndef f(convert=len):\n    convert(str(os.environ))\nf()\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\nhandlers = [len]\nhandlers[0](str(os.environ))\n'
        ))

    def test_sensitive_call_arguments_propagate_into_parameters(self) -> None:
        for body in (
            'import os\ndef lookup(key):\n    return {}[key]\nlookup(str(os.environ))\n',
            'import os\ndef lookup(key=None):\n    return {}[key]\nlookup(key=str(os.environ))\n',
            'import os\ndef lookup(*keys):\n    return {}[keys[0]]\nlookup(str(os.environ))\n',
            'import os\ndef outer():\n    secret = str(os.environ)\n'
            '    def inner():\n        return {}[secret]\n    return inner()\nouter()\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect(
            'import os\ndef lookup(key):\n    return {}[key]\nlookup("name")\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\ndef lookup(key):\n    return {}[key]\n'
        ))

    def test_late_inserted_sensitive_keys_lose_keys_only_exemption(self) -> None:
        for body in (
            'import os\nd = {}\nd[str(os.environ)] = 1\nfor key in d:\n    {}[key]\n',
            'import os\nd = {}\nd.update({str(os.environ): 1})\nfor key in d:\n    {}[key]\n',
            'import os\nd = {}\nd.setdefault(str(os.environ), 1)\nfor key in d:\n    {}[key]\n',
            'import os\nd = {}\nd |= {str(os.environ): 1}\nfor key in d:\n    {}[key]\n',
            'import os\nd = {}\nd.__setitem__(str(os.environ), 1)\nfor key in d:\n    {}[key]\n',
            'import os\nd = {}\nd.update([(str(os.environ), 1)])\nfor key in d:\n    {}[key]\n',
            'import os\nfor ch in str(os.environ):\n    {}[ch]\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect(
            'import os\nd = {}\nd["static"] = 1\nfor key in d:\n    {}[key]\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\nd = {}\nd.update(os.environ)\nfor key in d:\n    {}[key]\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\nd = {}\nd.update(host="x")\nfor key in d:\n    {}[key]\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\ndef f():\n    d = {}\n    d[str(os.environ)] = 1\n'
            'def g():\n    d = {}\n    for key in d:\n        {}[key]\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\nfor key in sorted(os.environ):\n    {}[key]\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\nfor key in list(os.environ):\n    {}[key]\n'
        ))

    def test_keys_with_sensitive_representations_are_rejected(self) -> None:
        for body in (
            'import os\nclass K:\n    def __repr__(self):\n'
            '        return str(os.environ)\n{}[K()]\n',
            'import os\nclass K:\n    def __repr__(self):\n'
            '        return str(os.environ)\nk = K()\n{}[k]\n',
            'import os\nclass K:\n    def __repr__(self):\n'
            '        return str(os.environ)\nclass K2(K):\n    pass\n{}[K2()]\n',
            'import os\nclass K:\n    def __repr__(self):\n'
            '        return str(os.environ)\ndef f(k):\n    return {}[k]\nf(K())\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))
        self.assertIsNone(self.inspect(
            'class K:\n    pass\n{}[K()]\n'
        ))
        self.assertIsNone(self.inspect(
            'class K:\n    def __repr__(self):\n        return "K()"\n{}[K()]\n'
        ))
        self.assertIsNone(self.inspect(
            'def make():\n    return "k"\n{}[make()]\n'
        ))
        self.assertIsNone(self.inspect(
            'class F:\n    def build(self):\n        return "k"\n{}[F().build()]\n'
        ))

    def test_awk_program_cannot_rewrite_reader_argv(self) -> None:
        self.assertIsNotNone(self.shell_violation(
            'awk \'BEGIN { ARGV[1]="maintainer.pem" } {print}\' docs/EXECUTION.md'
        ))
        self.assertIsNone(self.shell_violation("awk '{print}' docs/EXECUTION.md"))

    def test_shell_reader_requires_explicit_reviewed_operand(self) -> None:
        self.assertIsNotNone(self.shell_violation('grep -e . -- -maintainer.pem'))
        self.assertIsNotNone(self.shell_violation('rg --hidden --no-ignore .'))
        self.assertIsNotNone(self.shell_violation('rg --hidden --no-ignore . .'))
        self.assertIsNotNone(self.shell_violation('grep -R . .'))
        self.assertIsNotNone(self.shell_violation('grep -d recurse . .'))
        self.assertIsNotNone(self.shell_violation('grep --directories=recurse . .'))
        self.assertIsNotNone(self.shell_violation('grep --direct=recurse . .'))
        self.assertIsNotNone(self.shell_violation('grep --direct recurse . .'))
        self.assertIsNone(self.shell_violation('rg . docs/EXECUTION.md'))
        self.assertIsNone(self.shell_violation('grep -R . docs/'))
        self.assertIsNone(self.shell_violation('grep -d recurse . docs/'))
        self.assertIsNone(self.shell_violation('rg -n . <<< reviewed'))

    def test_jq_environment_object_references_are_rejected(self) -> None:
        for command in (
            "jq -n '$ENV'",
            "jq -n '$ENV.GH_TOKEN'",
        ):
            with self.subTest(command=command):
                self.assertIsNotNone(self.shell_violation(command))

        self.assertIsNone(self.shell_violation('jq -n \'"reviewed"\''))

    def test_jq_external_module_loading_is_rejected(self) -> None:
        self.assertIsNotNone(self.shell_violation('jq -n -L/tmp \'include "evil"; leak\''))
        self.assertIsNotNone(self.shell_violation('jq -n \'include "evil"; leak\''))
        self.assertIsNone(self.shell_violation('jq -n \'"reviewed"\''))

    def test_path_getattr_readers_follow_local_path_and_member_returns(self) -> None:
        unsafe = (
            'from pathlib import Path\n'
            'def private_path():\n'
            '    return Path("synthetic-private/file")\n'
            'def reader_name():\n'
            '    return "read_text"\n'
            'reader = getattr(private_path(), reader_name())\n'
            'print(reader())\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'from pathlib import Path\n'
            'def reader_name():\n'
            '    return "read_text"\n'
            'reader = getattr(Path("docs/evidence/g01-recovery-packet.md"), reader_name())\n'
            'print(reader())\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_getattr_mapping_lookup_alias_preserves_launcher_provenance(self) -> None:
        unsafe = (
            'import subprocess\n'
            'launchers = {"x": subprocess.run}\n'
            'lookup = getattr(launchers, "get")\n'
            'launch = lookup("x")\n'
            'launch(["gh", "workflow", "run", "ci.yml"])\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

        safe = (
            'callbacks = {"upper": str.upper}\n'
            'lookup = getattr(callbacks, "get")\n'
            'transform = lookup("upper")\n'
            'transform("reviewed")\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_mapping_lookup_alias_tracking_respects_function_scopes(self) -> None:
        safe = (
            'import subprocess\n'
            'launchers = {"x": subprocess.run}\n'
            'def unused_launcher_lookup():\n'
            '    lookup = launchers.get\n'
            'def safe_local_lookup():\n'
            '    lookup = str.upper\n'
            '    lookup("reviewed")\n',
        )
        for body in safe:
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

        unsafe = (
            'import subprocess\n'
            'launchers = {"x": subprocess.run}\n'
            'def launcher_lookup():\n'
            '    lookup = getattr(launchers, "get")\n'
            '    lookup("x")\n',
        )
        for body in unsafe:
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

    def test_reviewed_evidence_path_assignment_must_be_unique(self) -> None:
        duplicate = ast.parse(
            'issue79_reviewed_evidence_paths = ("canonical",)\n'
            'issue79_reviewed_evidence_paths = ("later",)\n',
            filename="<duplicate-evidence-path-data>",
        )
        with self.assertRaises(AssertionError):
            _top_level_assignment(duplicate, "issue79_reviewed_evidence_paths")

    def test_pr103_lambda_method_error_arguments(self) -> None:
        for body in (
            '(lambda key: {}[key])(str(os.environ))\n',
            'lookup = lambda key: {}[key]\nlookup(str(os.environ))\n',
            'class C:\n    def lookup(self, key):\n        return {}[key]\nC().lookup(str(os.environ))\n',
            'class C:\n    @staticmethod\n    def lookup(key):\n        return {}[key]\nC.lookup(str(os.environ))\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect('import os\n' + body))
        self.assertIsNone(self.inspect('(lambda key: {}[key])("reviewed")\n'))

    def test_pr103_mapping_alias_mutations(self) -> None:
        for mutation in (
            'a[str(os.environ)] = 1',
            'a.update({str(os.environ): 1})',
            'a.setdefault(str(os.environ), 1)',
            'a.__setitem__(str(os.environ), 1)',
            'a |= {str(os.environ): 1}',
        ):
            with self.subTest(mutation=mutation):
                self.assertIsNotNone(self.inspect(
                    'import os\nd = {}\na = d\nb = a\na = b\n' + mutation + '\nfor key in d:\n    {}[key]\n'
                ))
        self.assertIsNone(self.inspect('d = {}\na = d\na["reviewed"] = 1\nfor key in d:\n    {}[key]\n'))

    def test_pr103_dictionary_conversion_callables(self) -> None:
        for conversion in ('int', 'float', 'complex'):
            with self.subTest(conversion=conversion):
                self.assertIsNotNone(self.inspect(
                    f'import os\nconverters = {{"x": {conversion}}}\nconverters["x"](str(os.environ))\n'
                ))
        self.assertIsNone(self.inspect('converters = {"x": complex}\nconverters["x"]("1")\n'))
        self.assertIsNone(self.inspect('import os\nconverters = {"x": complex, "safe": len}\nconverters["safe"](str(os.environ))\n'))

    def test_pr103_factory_returned_repr_keys(self) -> None:
        for result in ('K()', 'make()'):
            with self.subTest(result=result):
                self.assertIsNotNone(self.inspect(
                    'import os\nclass K:\n    def __repr__(self):\n        return str(os.environ)\n'
                    'def make():\n    return K()\ndef again():\n    return ' + result + '\n{}[again()]\n'
                ))
        self.assertIsNone(self.inspect(
            'class K:\n    def __repr__(self):\n        return "reviewed"\ndef make():\n    return K()\n{}[make()]\n'
        ))

    def test_pr103_comprehension_launcher_defaults(self) -> None:
        for default, call in (
            ('[x for x in [os.system]]', 'box[0]("synthetic-command")'),
            ('{x for x in [os.system]}', 'next(iter(box))("synthetic-command")'),
            ('(x for x in [os.system])', 'next(box)("synthetic-command")'),
            ('{"x": x for x in [os.system]}', 'box["x"]("synthetic-command")'),
            ('[os.system for x in [0]]', 'box[0]("synthetic-command")'),
            ('[x for x in [subprocess.run]]', 'box[0](["gh", "workflow", "run", "synthetic"])'),
        ):
            with self.subTest(default=default):
                self.assertIsNotNone(self.inspect(
                    'import os, subprocess\ndef f(box=' + default + '):\n    ' + call + '\nf()\n'
                ))
        self.assertIsNone(self.inspect('def f(box=[x for x in [len]]):\n    box[0]("reviewed")\nf()\n'))

    def test_pr103_sensitive_attribute_names(self) -> None:
        for lookup in ('getattr', 'delattr'):
            with self.subTest(lookup=lookup):
                self.assertIsNotNone(self.inspect(f'import os\n{lookup}(object(), str(os.environ))\n'))
                self.assertIsNotNone(self.inspect(f'import os\nlookup = {lookup}\nlookup(object(), str(os.environ))\n'))
        self.assertIsNone(self.inspect('getattr(object(), "reviewed", None)\n'))

    def test_pr103_class_namespace_launchers(self) -> None:
        for member, call in (
            ('from os import system as go', 'C.go("synthetic-command")'),
            ('import os as tools', 'C.tools.system("synthetic-command")'),
            ('from subprocess import run as go', 'C.go(["gh", "workflow", "run", "synthetic"])'),
        ):
            with self.subTest(member=member):
                self.assertIsNotNone(self.inspect('class C:\n    ' + member + '\n' + call + '\n'))
        self.assertIsNone(self.inspect('class C:\n    import re as tools\nprint(C.tools.escape("reviewed"))\n'))

    def test_pr103_display_methods_at_sinks(self) -> None:
        for method in ('__str__', '__repr__', '__format__'):
            for sink in ('print(K())', 'sys.exit(K())', 'print(make())', 'print(str(K()))', 'print(f"{K()}")'):
                with self.subTest(method=method, sink=sink):
                    self.assertIsNotNone(self.inspect(
                        'import os, sys\nclass K:\n    def ' + method + '(self, *args):\n        return str(os.environ)\n'
                        'def make():\n    return K()\n' + sink + '\n'
                    ))
        self.assertIsNone(self.inspect('class K:\n    def __str__(self):\n        return "reviewed"\nprint(K())\n'))

    def test_pr103_display_constructor_aliases(self) -> None:
        self.assertIsNotNone(self.inspect(
            'import os\nclass K:\n    def __str__(self):\n        return str(os.environ)\nAlias = K\nprint(Alias())\n'
        ))
        self.assertIsNone(self.inspect(
            'class K:\n    def __str__(self):\n        return "reviewed"\nAlias = K\nprint(Alias())\n'
        ))

    def test_pr103_unpacked_dictionary_converters(self) -> None:
        self.assertIsNotNone(self.inspect(
            'import os\nconverters = {**{"x": complex}}\nconverters["x"](str(os.environ))\n'
        ))
        self.assertIsNone(self.inspect(
            'import os\nconverters = {**{"x": len}}\nconverters["x"](str(os.environ))\n'
        ))

    def test_pr103_delta_unbound_method_arguments(self) -> None:
        for parameters, call in (("self, key", 'C.lookup(None, str(os.environ))'),
                                 ("self, key", 'C.lookup(C(), str(os.environ))'),
                                 ("self", 'C.lookup(str(os.environ))')):
            key = "key" if "," in parameters else "self"
            with self.subTest(call=call):
                self.assertIsNotNone(self.inspect('import os\nclass C:\n    def lookup(' + parameters + '):\n        return {}[' + key + ']\n' + call + '\n'))
        for call in ('C.lookup(None, "safe")', 'C().lookup("safe")'):
            self.assertIsNone(self.inspect('class C:\n    def lookup(self, key):\n        return {}[key]\n' + call + '\n'))
        self.assertIsNone(self.inspect('class C:\n    @staticmethod\n    def lookup(key):\n        return {}[key]\nC.lookup("safe")\n'))

    def test_pr103_delta_mapping_identity(self) -> None:
        for mutation in ('a[str(os.environ)] = 1', 'a.update({str(os.environ): 1})',
                         'a.setdefault(str(os.environ), 1)', 'a.__setitem__(str(os.environ), 1)',
                         'a |= {str(os.environ): 1}'):
            with self.subTest(mutation=mutation):
                self.assertIsNotNone(self.inspect('import os\nd = {}\ndef mutate(a):\n    ' + mutation + '\nmutate(d)\nfor key in d:\n    {}[key]\n'))
        for receiver in ('d', 'alias'):
            with self.subTest(receiver=receiver):
                alias = '        alias = d\n' if receiver == 'alias' else ''
                self.assertIsNotNone(self.inspect('import os\ndef outer():\n    d = {}\n    def mutate():\n' + alias + '        ' + receiver + '[str(os.environ)] = 1\n    mutate()\n    for key in d:\n        {}[key]\nouter()\n'))
        self.assertIsNotNone(self.inspect('import os\nd = {}\ndef mutate(a):\n    alias = a\n    alias[str(os.environ)] = 1\nmutate(d)\nfor key in d:\n    {}[key]\n'))
        self.assertIsNone(self.inspect('import os\ndef mutate(a):\n    a[str(os.environ)] = 1\nd = {}\nother = {}\nmutate(other)\nfor key in d:\n    {}[key]\n'))
        self.assertIsNone(self.inspect('import os\ndef other():\n    d = {}\n    d[str(os.environ)] = 1\ndef safe():\n    d = {}\n    for key in d:\n        {}[key]\nsafe()\n'))

    def test_pr103_delta_named_comprehension_defaults(self) -> None:
        for expression, call in (('[x for x in [os.system]]', 'arg[0]("synthetic-command")'),
                                 ('{x for x in [os.system]}', 'next(iter(arg))("synthetic-command")'),
                                 ('(x for x in [os.system])', 'next(arg)("synthetic-command")'),
                                 ('{"x": x for x in [os.system]}', 'arg["x"]("synthetic-command")')):
            with self.subTest(expression=expression):
                self.assertIsNotNone(self.inspect('import os\nbox = ' + expression + '\nalias = box\ndef f(arg=alias):\n    ' + call + '\nf()\n'))
        self.assertIsNone(self.inspect('box = [x for x in [len]]\nalias = box\ndef f(arg=alias):\n    print(arg[0]("safe"))\nf()\n'))

    def test_pr103_delta_display_subscripts(self) -> None:
        for expression in ('Alias = (K,)[0]\nprint(Alias())',
                           'classes = (K,)\nAlias = classes[0]\nprint(Alias())',
                           'box = [K()]\nprint(box[0])',
                           'box = {"x": K()}\nalias = box\nprint(alias["x"])'):
            for sensitive in (True, False):
                with self.subTest(expression=expression, sensitive=sensitive):
                    result = 'str(os.environ)' if sensitive else '"safe"'
                    violation = self.inspect('import os\nclass K:\n    def __str__(self):\n        return ' + result + '\n' + expression + '\n')
                    if sensitive:
                        self.assertIsNotNone(violation)
                    else:
                        self.assertIsNone(violation)

    def test_pr103_delta_conversion_dictionary_aliases(self) -> None:
        for conversion in ('int', 'float', 'complex'):
            for container in ('a', '{**a}'):
                with self.subTest(conversion=conversion, container=container):
                    prefix = 'import os\na = {"x": ' + conversion + ', "safe": len}\nb = ' + container + '\nc = b\n'
                    self.assertIsNotNone(self.inspect(prefix + 'c["x"](str(os.environ))\n'))
                    self.assertIsNone(self.inspect(prefix + 'c["x"]("1")\n'))
                    if container == 'a':
                        self.assertIsNone(self.inspect(prefix + 'c["safe"](str(os.environ))\n'))

    def test_pr103_delta_class_import_receivers(self) -> None:
        for imported in ('from os import system as go', 'from subprocess import run as go'):
            for receiver in ('D', 'C()', 'instance'):
                with self.subTest(imported=imported, receiver=receiver):
                    self.assertIsNotNone(self.inspect('class C:\n    ' + imported + '\nD = C\ninstance = C()\n' + receiver + '.go("synthetic-command")\n'))
        for receiver in ('D', 'C()', 'instance'):
            self.assertIsNone(self.inspect('class C:\n    from re import escape as go\nD = C\ninstance = C()\nprint(' + receiver + '.go("safe"))\n'))

    def test_pr103_delta_subscript_cycles_retain_alternatives(self) -> None:
        for bindings in ('box = [complex]\nbox = box[0]\n',
                         'box = other[0]\nother = box\nbox = [complex]\n'):
            with self.subTest(bindings=bindings):
                self.assertIsNotNone(self.inspect('import os\n' + bindings + 'box[0](str(os.environ))\n'))
        self.assertIsNone(self.inspect('box = other[0]\nother = box\nbox = [len]\nprint(box[0]("safe"))\n'))

    def test_pr103_gh_setattr_names(self) -> None:
        for target in ('setattr', 'setter'):
            self.assertIsNotNone(self.inspect('import os\nsetter = setattr\n' + target + '(object(), str(os.environ), 1)\n'))
        self.assertIsNone(self.inspect('class C: pass\nsetattr(C(), "safe", 1)\n'))

    def test_pr103_gh_nested_class_launchers(self) -> None:
        for suite in ('    if True:\n        ', '    for item in [1]:\n        ',
                      '    try:\n        ', '    with contextlib.nullcontext():\n        '):
            ending = '    finally:\n        pass\n' if 'try:' in suite else ''
            with self.subTest(suite=suite):
                self.assertIsNotNone(self.inspect('import contextlib\nclass C:\n' + suite + 'from os import system as go\n' + ending + 'C.go("synthetic-command")\n'))
        self.assertIsNone(self.inspect('class C:\n    if True:\n        from re import escape as go\nprint(C.go("safe"))\n'))

    def test_pr103_gh_destructured_mapping_aliases(self) -> None:
        for alias in ('a, = (d,)', '[a] = [d]', '(a, [b]) = (d, [d])'):
            self.assertIsNotNone(self.inspect('import os\nd = {}\n' + alias + '\na[str(os.environ)] = 1\nfor key in d:\n    {}[key]\n'))
        self.assertIsNone(self.inspect('d = {}\na, = (d,)\na["safe"] = 1\nfor key in d:\n    {}[key]\n'))

    def test_pr103_gh_inserted_conversion_callables(self) -> None:
        for conversion in ('int', 'float', 'complex'):
            for store in ('d["x"] = CONVERT', 'd.update({"x": CONVERT})',
                          'd.setdefault("x", CONVERT)', 'd.__setitem__("x", CONVERT)',
                          'd |= {"x": CONVERT}'):
                prefix = 'import os\nd = {}\n' + store.replace('CONVERT', conversion) + '\na = d\n'
                with self.subTest(conversion=conversion, store=store):
                    self.assertIsNotNone(self.inspect(prefix + 'a["x"](str(os.environ))\n'))
                    self.assertIsNone(self.inspect(prefix + 'a["x"]("1")\n'))
        self.assertIsNone(self.inspect('import os\nd = {}\nd["x"] = complex\nd["safe"] = len\nd["safe"](str(os.environ))\n'))

    def test_pr103_gh_assigned_display_methods(self) -> None:
        for implementation in ('def leak(self):\n    return RESULT\n', 'leak = lambda self: RESULT\n'):
            for install in ('class K:\n    __str__ = leak\n', 'class K: pass\nK.__str__ = leak\n'):
                for sensitive in (True, False):
                    code = 'import os\n' + implementation.replace('RESULT', 'str(os.environ)' if sensitive else '"safe"') + install + 'print(K())\n'
                    with self.subTest(implementation=implementation, install=install, sensitive=sensitive):
                        if sensitive:
                            self.assertIsNotNone(self.inspect(code))
                        else:
                            self.assertIsNone(self.inspect(code))

    def test_pr103_gh_bound_and_percent_display(self) -> None:
        for protocol, expression in (('__format__', '"{}".format(K())'),
                                     ('__str__', '"%s" % K()'), ('__repr__', '"%r" % K()')):
            for sensitive in (True, False):
                code = 'import os\nclass K:\n    def ' + protocol + '(self, *args):\n        return ' + ('str(os.environ)' if sensitive else '"safe"') + '\nprint(' + expression + ')\n'
                with self.subTest(protocol=protocol, sensitive=sensitive):
                    if sensitive:
                        self.assertIsNotNone(self.inspect(code))
                    else:
                        self.assertIsNone(self.inspect(code))

    def test_pr103_gh_implicit_path_errors(self) -> None:
        for expression in ('Path.cwd().relative_to("/definitely-no")', 'root.relative_to("/definitely-no")'):
            self.assertIsNotNone(self.inspect('from pathlib import Path\nroot = Path.cwd()\n' + expression + '\n'))
        self.assertIsNone(self.inspect('from pathlib import Path\nPath("safe/child").relative_to("safe")\n'))
        self.assertIsNone(self.inspect('from pathlib import Path\ntry:\n    Path.cwd().relative_to("/definitely-no")\nexcept ValueError:\n    pass\n'))
        self.assertIsNotNone(self.inspect('from pathlib import Path\ntry:\n    Path.cwd().relative_to("/definitely-no")\nexcept ValueError:\n    raise\n'))
        self.assertIsNotNone(self.inspect('from pathlib import Path\ntry:\n    Path.cwd().relative_to("/definitely-no")\nexcept ValueError as error:\n    print(error)\n'))
        self.assertIsNotNone(self.inspect('from pathlib import Path\ntry:\n    Path.cwd().relative_to("/definitely-no")\nexcept ValueError:\n    raise RuntimeError("safe")\n'))
        self.assertIsNone(self.inspect('from pathlib import Path\ntry:\n    Path.cwd().relative_to("/definitely-no")\nexcept ValueError:\n    raise SystemExit("safe")\n'))
        self.assertIsNone(self.inspect('from pathlib import Path\ntry:\n    Path.cwd().relative_to("/definitely-no")\nexcept ValueError:\n    raise RuntimeError("safe") from None\n'))

    def test_pr103_gh_helper_launcher_defaults(self) -> None:
        for launcher in ('os.system', 'len'):
            code = 'import os\ndef choose():\n    return ' + launcher + '\ndef f(launch=choose()):\n    launch("synthetic-command")\nf()\n'
            if launcher == 'os.system':
                self.assertIsNotNone(self.inspect(code))
            else:
                self.assertIsNone(self.inspect(code))

    def test_pr103_gh_wrapped_open_readers(self) -> None:
        for expression in ('(open,)[0]', '{"reader": open}["reader"]', 'True and open', 'choose()'):
            code = 'def choose():\n    return open\nreader = ' + expression + '\nprint(reader("/tmp/synthetic-private").read())\n'
            with self.subTest(expression=expression):
                self.assertIsNotNone(self.inspect(code))
        self.assertIsNone(self.inspect('reader = (len,)[0]\nprint(reader("safe"))\n'))
        self.assertIsNone(self.inspect('reader = (len, open)[0]\nprint(reader("safe"))\n'))
        self.assertIsNone(self.inspect('reader = {"safe": len, "reader": open}["safe"]\nprint(reader("safe"))\n'))

    def test_pr103_gh_class_sensitive_imports(self) -> None:
        for member in ('environ', 'environb'):
            for receiver in ('C', 'D', 'C()'):
                with self.subTest(member=member, receiver=receiver):
                    self.assertIsNotNone(self.inspect('class C:\n    from os import ' + member + ' as inherited\nD = C\nprint(' + receiver + '.inherited)\n'))
        self.assertIsNone(self.inspect('class C:\n    from re import escape as inherited\nprint(C.inherited("safe"))\n'))

    def test_pr103_gh_adjacent_update_sources(self) -> None:
        for store in ('d.update(x=complex)', 'updates = {"x": complex}\nd.update(updates)'):
            prefix = 'import os\nd = {}\n' + store + '\n'
            with self.subTest(store=store):
                self.assertIsNotNone(self.inspect(prefix + 'd["x"](str(os.environ))\n'))
                self.assertIsNone(self.inspect(prefix + 'd["x"]("1")\n'))

    def test_pr103_gh_adjacent_formatter_alias(self) -> None:
        for sensitive in (True, False):
            code = 'import os\nclass K:\n    def __format__(self, spec):\n        return ' + ('str(os.environ)' if sensitive else '"safe"') + '\nformatter = "{}".format\nprint(formatter(K()))\n'
            with self.subTest(sensitive=sensitive):
                if sensitive:
                    self.assertIsNotNone(self.inspect(code))
                else:
                    self.assertIsNone(self.inspect(code))

    def test_pr103_review_handler_order(self) -> None:
        prefix = 'from pathlib import Path\ntry:\n    Path.cwd().relative_to("/definitely-no")\n'
        for first in ('except Exception as err:\n    print(err)\n',
                      'except (TypeError, ValueError) as err:\n    print(err)\n',
                      'except BaseException as err:\n    print(err)\n'):
            with self.subTest(first=first):
                self.assertIsNotNone(self.inspect(prefix + first + 'except ValueError:\n    pass\n'))
        self.assertIsNone(self.inspect(prefix + 'except TypeError:\n    {}["safe"]\nexcept ValueError:\n    pass\n'))
        self.assertIsNone(self.inspect(prefix + 'except ValueError:\n    pass\nexcept Exception as err:\n    print(err)\n'))

    def test_pr103_review_handler_suppression(self) -> None:
        prefix = ('from pathlib import Path\ndef fail():\n    raise RuntimeError("safe")\n'
                  'try:\n    Path.cwd().relative_to("/definitely-no")\nexcept ValueError:\n')
        for effect in ('{}["safe"]', '1 / 0', 'fail()',
                       'raise RuntimeError({}["safe"]) from None',
                       'try:\n        raise RuntimeError("safe") from None\n    finally:\n        1 / 0'):
            with self.subTest(effect=effect):
                self.assertIsNotNone(self.inspect(prefix + '    ' + effect + '\n'))
        for effect in ('pass', 'raise RuntimeError("safe") from None', 'raise SystemExit("safe")'):
            with self.subTest(safe_effect=effect):
                self.assertIsNone(self.inspect(prefix + '    ' + effect + '\n'))
        self.assertIsNotNone(self.inspect('from pathlib import Path\ndef f():\n    try:\n        Path.cwd().relative_to("/definitely-no")\n    except ValueError:\n        return {}["safe"]\nf()\n'))

    def test_pr103_review_starred_mapping_identity(self) -> None:
        for binding in ('a, *rest, b = (other, other, other, d)',
                        '*rest, b = (other, other, d)',
                        '(a, *rest, b), = ((other, other, d),)',
                        'a, *rest, (b,) = (other, other, (d,))',
                        'a, *rest, b = (other, d, *empty)'):
            for key in ('str(os.environ)', '"safe"'):
                code = 'import os\nd = {}\nother = {}\nempty = ()\n' + binding + '\nb[' + key + '] = 1\nfor key in d:\n    {}[key]\n'
                with self.subTest(binding=binding, key=key):
                    if key == '"safe"':
                        self.assertIsNone(self.inspect(code))
                    else:
                        self.assertIsNotNone(self.inspect(code))
        self.assertIsNotNone(self.inspect('import os\nd = {}\nother = {}\na, *rest, b = (d, other, other)\na[str(os.environ)] = 1\nfor key in d:\n    {}[key]\n'))

    def test_pr103_review_handler_adjacent_proof(self) -> None:
        prefix = 'from pathlib import Path\ntry:\n    Path.cwd().relative_to("/definitely-no")\n'
        for handlers in ('except (missing, ValueError):\n    pass\n',
                         'except ValueError:\n    raise SystemExit("safe")\nfinally:\n    1 / 0\n',
                         'except ValueError:\n    raise RuntimeError(f"{missing}") from None\n'):
            with self.subTest(handlers=handlers):
                self.assertIsNotNone(self.inspect(prefix + handlers))
        for setup in ('if False:\n    msg = "safe"\n', 'msg = "safe"\ndel msg\n',
                      'args = sys.argv\nargs[0] = object()\nmsg = sys.argv[0]\n'):
            with self.subTest(setup=setup):
                self.assertIsNotNone(self.inspect('import sys\n' + setup + prefix
                    + 'except ValueError:\n    raise SystemExit(f"{msg}")\n'))
        self.assertIsNotNone(self.inspect(prefix
            + 'except ValueError:\n    raise RuntimeError(msg) from None\nmsg = "safe"\n'))
        self.assertIsNone(self.inspect('msg = "safe"\n' + prefix
            + 'except ValueError:\n    raise RuntimeError(f"{msg}") from None\n'))

    def test_pr103_review_assigned_display_provenance(self) -> None:
        for expression in ('make()', '(leak,)[0]', 'box["display"]', 'again()'):
            for install in ('class K:\n    __str__ = IMPL\n', 'class K: pass\nK.__str__ = IMPL\n'):
                for result in ('str(os.environ)', '"safe"'):
                    code = ('import os\ndef leak(self):\n    return ' + result + '\n'
                            'def make():\n    return leak\ndef again():\n    return make()\n'
                            'box = {"display": leak}\n' + install.replace('IMPL', expression) + 'print(K())\n')
                    with self.subTest(expression=expression, install=install, result=result):
                        if result == '"safe"':
                            self.assertIsNone(self.inspect(code))
                        else:
                            self.assertIsNotNone(self.inspect(code))

    def test_pr103_recheck_rhs_starred_mapping(self) -> None:
        for binding in ('a, *rest, b = (other, *bundle)',
                        '(a, *rest, b), = ((other, *bundle),)',
                        'alias = bundle\nbundle = alias\na, *rest, b = (other, *alias)'):
            for key in ('str(os.environ)', '"safe"'):
                code = ('import os\nd = {}\nother = {}\nbundle = (other, d)\n'
                        + binding + '\nb[' + key + '] = 1\nfor key in d:\n    {}[key]\n')
                with self.subTest(binding=binding, key=key):
                    if key == '"safe"':
                        self.assertIsNone(self.inspect(code))
                    else:
                        self.assertIsNotNone(self.inspect(code))

    def test_pr103_recheck_handler_message_binders(self) -> None:
        prefix = 'from pathlib import Path\nmsg = "safe"\n'
        body = 'try:\n    Path.cwd().relative_to("/definitely-no")\nexcept ValueError'
        for replacement in ('SystemExit(msg)', 'RuntimeError(f"{msg}") from None'):
            with self.subTest(replacement=replacement):
                self.assertIsNotNone(self.inspect(prefix + body + ' as msg:\n    raise ' + replacement + '\n'))
        for binder in ('try:\n    1 / 0\nexcept ZeroDivisionError as msg:\n    pass\n',
                       'with object() as msg:\n    pass\n',
                       'for msg in [object()]:\n    pass\n',
                       'match object():\n    case msg:\n        pass\n',
                       'import re as msg\n'):
            with self.subTest(binder=binder):
                self.assertIsNotNone(self.inspect(prefix + binder + body
                    + ':\n    raise RuntimeError(f"{msg}") from None\n'))
        self.assertIsNone(self.inspect(prefix + body + ':\n    raise RuntimeError(f"{msg}") from None\n'))

    def test_pr103_recheck_enclosing_cleanup_context(self) -> None:
        for replacement in ('SystemExit("safe")', 'RuntimeError("safe") from None'):
            for effect in ('pass', '1 / 0'):
                code = ('from pathlib import Path\ntry:\n    try:\n'
                        '        Path.cwd().relative_to("/definitely-no")\n'
                        '    except ValueError:\n        raise ' + replacement
                        + '\nfinally:\n    ' + effect + '\n')
                with self.subTest(replacement=replacement, effect=effect):
                    if replacement.startswith('SystemExit') and effect != 'pass':
                        self.assertIsNotNone(self.inspect(code))
                    else:
                        self.assertIsNone(self.inspect(code))
        self.assertIsNotNone(self.inspect('from pathlib import Path\ntry:\n    try:\n        Path.cwd().relative_to("/definitely-no")\n    except ValueError:\n        raise SystemExit("safe")\nexcept BaseException:\n    raise RuntimeError("replacement")\n'))

    def test_pr103_recheck_replacement_constructor_bindings(self) -> None:
        for setup, handler in (('from re import compile as RuntimeError\n', 'except ValueError'),
                               ('', 'except ValueError as RuntimeError')):
            with self.subTest(setup=setup, handler=handler):
                self.assertIsNotNone(self.inspect('from pathlib import Path\n' + setup
                    + 'try:\n    Path.cwd().relative_to("/definitely-no")\n'
                    + handler + ':\n    raise RuntimeError("[") from None\n'))
        self.assertIsNone(self.inspect('from pathlib import Path\ntry:\n    Path.cwd().relative_to("/definitely-no")\nexcept ValueError:\n    raise RuntimeError("[") from None\n'))

    def test_pr103_final_lambda_sensitive_defaults(self) -> None:
        for definition in ('f = lambda value=VALUE: print(value)',
                           'f = lambda *, value=VALUE: print(value)'):
            for value in ('os.environ', '"safe"'):
                with self.subTest(definition=definition, value=value):
                    violation = self.inspect('import os\n' + definition.replace('VALUE', value) + '\nf()\n')
                    if value == '"safe"':
                        self.assertIsNone(violation)
                    else:
                        self.assertIsNotNone(violation)

    def test_pr103_final_open_callable_defaults(self) -> None:
        for definition in ('def f(reader=SOURCE):\n    print(reader(PATH).read())',
                           'f = lambda reader=SOURCE: print(reader(PATH).read())',
                           'def f(*, reader=SOURCE):\n    print(reader(PATH).read())'):
            for path in ('"/tmp/synthetic-private"', '"docs/backlog.json"'):
                code = definition.replace('SOURCE', 'open').replace('PATH', path) + '\nf()\n'
                with self.subTest(definition=definition, path=path):
                    if path == '"docs/backlog.json"':
                        self.assertIsNone(self.inspect(code))
                    else:
                        self.assertIsNotNone(self.inspect(code))
        self.assertIsNone(self.inspect('def f(reader=len):\n    print(reader("safe"))\nf()\n'))

    def test_pr103_final_breakpoint_hook(self) -> None:
        for call in ('breakpoint("synthetic-command")', 'hook = breakpoint\nhook("synthetic-command")'):
            with self.subTest(call=call):
                self.assertIsNotNone(self.inspect('import os, sys\nsys.breakpointhook = os.system\n' + call + '\n'))
        self.assertIsNone(self.inspect('def breakpoint(value):\n    return len(value)\nprint(breakpoint("safe"))\n'))

    def test_pr103_final_custom_iterator_values(self) -> None:
        for implementation in ('__iter__ = lambda self: iter(VALUE)',
                               'def __iter__(self):\n        yield from VALUE',
                               'def __getitem__(self, index):\n        return VALUE[index]'):
            for value in ('os.environ.items()', '[("safe", "value")]'):
                for constructor in ('dict', 'list', 'tuple'):
                    code = ('import os\nclass K:\n    ' + implementation.replace('VALUE', value)
                            + '\nprint(' + constructor + '(K()))\n')
                    with self.subTest(implementation=implementation, value=value, constructor=constructor):
                        if value.startswith('os.'):
                            self.assertIsNotNone(self.inspect(code))
                        else:
                            self.assertIsNone(self.inspect(code))

    def test_pr103_final_iterator_adjacent_provenance(self) -> None:
        for implementation in ('def __iter__(self):\n        return self\n    def __next__(self):\n        return VALUE',
                               'def emit(self):\n    yield VALUE\nclass K:\n    __iter__ = emit'):
            for value in ('str(os.environ)', '"safe"'):
                code = 'import os\n' + (('class K:\n    ' + implementation) if implementation.startswith('def __iter__') else implementation).replace('VALUE', value) + '\ncollect = list\nprint(collect(K()))\n'
                with self.subTest(implementation=implementation, value=value):
                    if value == '"safe"':
                        self.assertIsNone(self.inspect(code))
                    else:
                        self.assertIsNotNone(self.inspect(code))

    def test_pr103_current_scanner_regex_binding_control(self) -> None:
        module = ast.parse(_scanner_module_source(PACKET_TEXT))
        fence = next(node for node in module.body
                     if isinstance(node, ast.FunctionDef) and node.name == "fence_details")
        calls = [node for node in ast.walk(fence)
                 if isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute)
                 and node.func.attr == "group"]
        self.assertTrue(calls)
        for call in calls:
            self.assertTrue(self.scanner["python_known_non_path_reader_call"](call, module))  # type: ignore[operator]

    def test_pr103_current_markdown_diagnostics_suppress_context(self) -> None:
        bodies = [body for _line, body, _marker, _invocation
                  in self.scanner["python_heredoc_bodies"](PACKET_TEXT)  # type: ignore[operator]
                  if 'def markdown_outside_fences(markdown):' in body
                  and 'local markdown link/anchor check: passed' in body]
        self.assertEqual(len(bodies), 1)
        module = ast.parse(bodies[0])
        parents = {child: parent for parent in ast.walk(module)
                   for child in ast.iter_child_nodes(parent)}
        checks = [node for node in ast.walk(module) if isinstance(node, ast.Try)
                  and len(node.body) == 1 and isinstance(node.body[0], ast.Expr)
                  and isinstance(node.body[0].value, ast.Call)
                  and isinstance(node.body[0].value.func, ast.Attribute)
                  and node.body[0].value.func.attr == "relative_to"]
        self.assertEqual(len(checks), 2)
        # Each speculative diagnostic is the last list entry. Failure continues
        # its own loop before pop; success removes only that entry, preserving
        # earlier failures and their order for every sequence of path results.
        for check in checks:
            owner = parents[check]
            self.assertIsInstance(owner, ast.For)
            position = owner.body.index(check)
            append = owner.body[position - 1]
            pop = owner.body[position + 1]
            self.assertIsInstance(append, ast.Expr)
            self.assertIsInstance(pop, ast.Expr)
            self.assertIsInstance(append.value, ast.Call)
            self.assertIsInstance(pop.value, ast.Call)
            self.assertEqual(ast.unparse(append.value.func), "errors.append")
            self.assertEqual(len(append.value.args), 1)
            self.assertEqual(ast.unparse(pop.value.func), "errors.pop")
            self.assertEqual(pop.value.args, [])
            self.assertEqual(pop.value.keywords, [])
            self.assertEqual(len(check.handlers), 1)
            self.assertEqual(ast.unparse(check.handlers[0].type), "ValueError")
            self.assertEqual(len(check.handlers[0].body), 1)
            self.assertIsInstance(check.handlers[0].body[0], ast.Continue)
            self.assertEqual(check.orelse, [])
            self.assertEqual(check.finalbody, [])
        self.assertIsNone(self.inspect(bodies[0]))

    def test_pr103_current_scanner_method_helper_is_not_launcher_alias(self) -> None:
        module = ast.parse(_scanner_module_source(PACKET_TEXT))
        _modules, functions, _unresolved = self.scanner["python_import_bindings"](module)  # type: ignore[operator]
        self.assertTrue("methods" not in functions, "local methods helper acquired launcher provenance")

    def test_pr103_nine_comprehension_protocols(self) -> None:
        for expression in ('[item for item in K()]', '{item for item in K()}',
                           '(item for item in K())', '{item: item for item in K()}'):
            for source in ('os.environ.items()', '[("safe", "value")]'):
                with self.subTest(expression=expression, source=source):
                    result = self.inspect('import os\nclass K:\n    __iter__ = lambda self: iter(' + source + ')\nprint(' + expression + ')\n')
                    self.assertEqual(result is None, source.startswith('['))

    def test_pr103_nine_setter_display(self) -> None:
        for setter in ('setattr', 'install'):
            for method in ('__str__', '__repr__', '__format__', '__iter__'):
                for value in ('str(os.environ)', '"safe"'):
                    with self.subTest(setter=setter, method=method, value=value):
                        sink = 'print(list(K()))' if method == '__iter__' else 'print(K())'
                        result = self.inspect('import os\ndef leak(self, *args):\n    return ' + value + '\nclass K: pass\ninstall = setattr\n' + setter + '(K, "' + method + '", leak)\n' + sink + '\n')
                        self.assertEqual(result is None, value == '"safe"')

    def test_pr103_nine_open_comprehensions(self) -> None:
        for expression, selection in (('[candidate for candidate in [open]]', 'readers[0]'),
                                      ('{"x": candidate for candidate in [open]}', 'readers["x"]')):
            for path in ('/tmp/maintainer-private', 'docs/backlog.json'):
                with self.subTest(expression=expression, path=path):
                    result = self.inspect('readers = ' + expression + '\nreader = ' + selection + '\nprint(reader("' + path + '").read())\n')
                    self.assertEqual(result is None, path.startswith('docs/'))
        self.assertIsNone(self.inspect('readers = [candidate for candidate in [len]]\nprint(readers[0]("safe"))\n'))

    def test_pr103_nine_bound_relative_to(self) -> None:
        for alias in ('method = Path.cwd().relative_to',
                      'box = [Path.cwd().relative_to]\nmethod = box[0]',
                      'def choose():\n    return Path.cwd().relative_to\nmethod = choose()'):
            with self.subTest(alias=alias):
                self.assertIsNotNone(self.inspect('from pathlib import Path\n' + alias + '\nmethod("/definitely-no")\n'))
                self.assertIsNone(self.inspect('from pathlib import Path\n' + alias + '\ntry:\n    method("/definitely-no")\nexcept ValueError:\n    pass\n'))
        self.assertIsNone(self.inspect('from pathlib import Path\nmethod = Path("relative").relative_to\nmethod("root")\n'))

    def test_pr103_nine_dunder_attribute_names(self) -> None:
        for method in ('__getattribute__', '__setattr__', '__delattr__'):
            for alias in (False, True):
                for name in ('str(os.environ)', '"safe"'):
                    with self.subTest(method=method, alias=alias, name=name):
                        extra = ', None' if method == '__setattr__' else ''
                        target = 'lookup' if alias else 'object.' + method
                        prefix = 'lookup = object.' + method + '\n' if alias else ''
                        result = self.inspect('import os\n' + prefix + target + '(object(), ' + name + extra + ')\n')
                        self.assertEqual(result is None, name == '"safe"')

    def test_pr103_nine_returned_attribute_launchers(self) -> None:
        for assignment in ('C.go = choose()', 'class D:\n    go = choose()\nC = D',
                           'box = [choose()]\nC.go = box[0]'):
            for value in ('os.system', 'len'):
                with self.subTest(assignment=assignment, value=value):
                    result = self.inspect('import os\ndef choose():\n    return ' + value + '\nclass C: pass\n' + assignment + '\nC.go("rm -rf /tmp/maintainer-owned")\n')
                    self.assertEqual(result is None, value == 'len')

    def test_pr103_nine_breakpoint_aliases(self) -> None:
        for body in ('debug = breakpoint\ndebug()',
                     'box = [breakpoint]\ndebug = box[0]\ndebug()',
                     'def f(debug=breakpoint):\n    debug()\nf()',
                     'for debug in [breakpoint]:\n    debug()'):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body + '\n'))
        self.assertIsNone(self.inspect('def breakpoint():\n    return "safe"\ndebug = breakpoint\nprint(debug())\n'))

    def test_pr103_nine_encoder_taint(self) -> None:
        for expression in ('urllib.parse.quote(str(env))', 'shlex.quote(str(env))',
                           'urllib.parse.quote_plus(str(env))', 'urllib.parse.quote(str(env)).encode().hex()'):
            for value in ('os.environ', '"safe"'):
                with self.subTest(expression=expression, value=value):
                    result = self.inspect('import os, urllib.parse, shlex\nenv = ' + value + '\nprint(' + expression + ')\n')
                    self.assertEqual(result is None, value == '"safe"')

    def test_pr103_nine_format_map_fields(self) -> None:
        for call in ('("{" + key + "}").format_map({})',
                     'method = ("{" + key + "}").format_map\nmethod({})'):
            for value in ('str(env).encode().hex()', '"safe"'):
                with self.subTest(call=call, value=value):
                    result = self.inspect('import os\nenv = os.environ\nkey = ' + value + '\n' + call + '\n')
                    self.assertEqual(result is None, value == '"safe"')

    def test_pr103_nine_adjacent_bound_dunders(self) -> None:
        for method in ('__getattribute__', '__setattr__', '__delattr__'):
            for alias in (False, True):
                for value in ('str(os.environ)', '"safe"'):
                    with self.subTest(method=method, alias=alias, value=value):
                        prefix = 'lookup = object().' + method + '\n' if alias else ''
                        target = 'lookup' if alias else 'object().' + method
                        extra = ', None' if method == '__setattr__' else ''
                        result = self.inspect('import os\n' + prefix + target + '(' + value + extra + ')\n')
                        self.assertEqual(result is None, value == '"safe"')

    def test_pr103_nine_adjacent_encoder_aliases(self) -> None:
        for prefix in ('import urllib.parse\nencode = urllib.parse.quote',
                       'from urllib.parse import quote as encode',
                       'import urllib.parse\ntools = urllib.parse\nencode = tools.quote',
                       'import shlex\nencode = shlex.quote'):
            for value in ('os.environ', '"safe"'):
                with self.subTest(prefix=prefix, value=value):
                    result = self.inspect('import os\n' + prefix + '\nenv = ' + value + '\nprint(encode(str(env)))\n')
                    self.assertEqual(result is None, value == '"safe"')
        self.assertIsNone(self.inspect('from urllib.parse import quote as encode\ndef f():\n    def encode(value):\n        return "safe"\n    return encode(str(os.environ))\nimport os\nprint(f())\n'))

    def test_pr103_nine_adjacent_cycles(self) -> None:
        for source in ('breakpoint', 'len'):
            code = 'a = b\nb = a\nb = ' + source + '\na("safe")\n'
            with self.subTest(source=source):
                self.assertEqual(self.inspect(code) is None, source == 'len')
        self.assertIsNone(self.inspect('def outer():\n    debug = len\n    return debug("safe")\nprint(outer())\n'))

    def test_pr103_nine_adjacent_nested_and_loop_values(self) -> None:
        for source in ('breakpoint', 'len'):
            with self.subTest(source=source):
                result = self.inspect('box = [[' + source + ']]\ndebug = box[0][0]\ndebug("safe")\n')
                self.assertEqual(result is None, source == 'len')
        for source in ('open', 'len'):
            with self.subTest(reader=source):
                result = self.inspect('for reader in [' + source + ']:\n    print(reader("/tmp/maintainer-private"))\n')
                self.assertEqual(result is None, source == 'len')

    def test_pr103_nine_review_chained_assignments(self) -> None:
        for source in ('breakpoint', 'len'):
            with self.subTest(source=source):
                self.assertEqual(self.inspect('debug = other = ' + source + '\ndebug("safe")\n') is None, source == 'len')
        for value in ('str(os.environ)', '"safe"'):
            for code in ('import os\ndef leak(self):\n    return ' + value + '\nclass K: pass\ninstall = other = setattr\ninstall(K, "__str__", leak)\nprint(K())\n',
                         'import os, urllib.parse\nenv = ' + value + '\nencode = other = urllib.parse.quote\nprint(encode(str(env)))\n'):
                with self.subTest(value=value, code=code):
                    self.assertEqual(self.inspect(code) is None, value == '"safe"')

    def test_pr103_nine_review_class_members(self) -> None:
        for receiver in ('C', 'Alias', 'C()', 'instance'):
            for source in ('breakpoint', 'len'):
                with self.subTest(receiver=receiver, source=source):
                    self.assertEqual(self.inspect('class C:\n    debug = ' + source + '\nAlias = C\ninstance = C()\n' + receiver + '.debug("safe")\n') is None, source == 'len')
            for value in ('os.environ', '"safe"'):
                with self.subTest(receiver=receiver, encoder=value):
                    self.assertEqual(self.inspect('import os, urllib.parse\nclass C:\n    encode = urllib.parse.quote\nAlias = C\ninstance = C()\nenv = ' + value + '\nprint(' + receiver + '.encode(str(env)))\n') is None, value == '"safe"')
            for path in ('Path.cwd()', 'Path("relative")'):
                with self.subTest(receiver=receiver, path=path):
                    self.assertEqual(self.inspect('from pathlib import Path\nclass C:\n    method = ' + path + '.relative_to\nAlias = C\ninstance = C()\n' + receiver + '.method("/definitely-no")\n') is None, path == 'Path("relative")')
        for source in ('os.system', 'len'):
            with self.subTest(launcher=source):
                self.assertEqual(self.inspect('import os\ndef choose():\n    return ' + source + '\nclass C: pass\nC.go = choose()\nlaunch = C.go\nlaunch("synthetic-command")\n') is None, source == 'len')
        for source in ('breakpoint', 'len'):
            with self.subTest(cycle=source):
                self.assertEqual(self.inspect('class C: pass\nC.other = ' + source + '\nC.step = C.other\nC.other = C.step\nC.step("safe")\n') is None, source == 'len')

    def test_pr103_nine_review_destructured_iteration(self) -> None:
        for pattern, row in (('reader, label', '(SOURCE, "x")'),
                             ('(reader,), label', '((SOURCE,), "x")'),
                             ('*labels, reader', '("x", SOURCE)')):
            for source in ('open', 'len'):
                for code in ('for ' + pattern + ' in [' + row.replace('SOURCE', source) + ']:\n    print(reader("/tmp/synthetic-private").read())\n',
                             'readers = [reader for ' + pattern + ' in [' + row.replace('SOURCE', source) + ']]\nprint(readers[0]("/tmp/synthetic-private").read())\n'):
                    with self.subTest(pattern=pattern, source=source, code=code):
                        self.assertEqual(self.inspect(code) is None, source == 'len')
        self.assertIsNone(self.inspect('for reader, other in [(len, open)]:\n    print(reader("safe"))\n'))
        for source in ('open', 'len'):
            with self.subTest(rows_cycle=source):
                self.assertEqual(self.inspect('rows = other\nother = rows\nother = [(' + source + ', "x")]\nfor reader, label in rows:\n    print(reader("/tmp/synthetic-private").read())\n') is None, source == 'len')
        for source in ('breakpoint', 'len'):
            with self.subTest(debug=source):
                self.assertEqual(self.inspect('for debug, label in [(' + source + ', "x")]:\n    debug("safe")\n') is None, source == 'len')
        for target, rows in (('readers', '[[SOURCE]]'), ('*readers, label', '[(SOURCE, "x")]')):
            for source in ('open', 'len'):
                with self.subTest(target=target, source=source):
                    self.assertEqual(self.inspect('for ' + target + ' in ' + rows.replace('SOURCE', source) + ':\n    print(readers[0]("/tmp/synthetic-private").read())\n') is None, source == 'len')

    def test_pr103_nine_review_unbound_format_map(self) -> None:
        for target in ('str', 'Text'):
            for call in (target + '.format_map("{" + key + "}", {})',
                         'formatting = ' + target + '.format_map\nformatting("{" + key + "}", {})'):
                for value in ('str(env).encode().hex()', '"safe"'):
                    with self.subTest(call=call, value=value):
                        result = self.inspect('import os\nText = str\nenv = os.environ\nkey = ' + value + '\n' + call + '\n')
                        self.assertEqual(result is None, value == '"safe"')

    def test_pr103_nine_v2_extracted_encoder_calls(self) -> None:
        for container, expression in (('[urllib.parse.quote]', 'box[0]'), ('{"x": urllib.parse.quote}', 'box["x"]')):
            for value in ('os.environ', '"safe"'):
                with self.subTest(expression=expression, value=value):
                    self.assertEqual(self.inspect('import os, urllib.parse\nenv = ' + value + '\nbox = ' + container + '\nprint(' + expression + '(str(env)))\n') is None, value == '"safe"')
        self.assertIsNone(self.inspect('import os, urllib.parse\nenv = os.environ\nbox = {"safe": len, "encode": urllib.parse.quote}\nprint(box["safe"](str(env)))\n'))

    def test_pr103_nine_v2_supplied_class_parameters(self) -> None:
        for member, unsafe, safe, operation in (
            ('step', 'breakpoint', 'len', 'target.step()'),
            ('encode', 'urllib.parse.quote', 'urllib.parse.quote', 'print(target.encode(str(VALUE)))'),
            ('method', 'Path.cwd().relative_to', 'Path("relative").relative_to', 'target.method("/definitely-no")'),
            ('reader', 'open', 'len', 'print(target.reader("/tmp/synthetic-private").read())'),
        ):
            for invocation in ('invoke(C)', 'invoke(target=C)', 'invoke(*(C,))',
                               'invoke(**{"target": C})', 'pack = (C,)\ninvoke(*pack)',
                               'options = {"target": C}\ninvoke(**options)'):
                for sensitive in (True, False):
                    with self.subTest(member=member, invocation=invocation, sensitive=sensitive):
                        body = operation.replace('VALUE', 'env')
                        code = ('import os, urllib.parse\nfrom pathlib import Path\nenv = ' + ('os.environ' if sensitive else '"safe"') + '\nclass C:\n    ' + member + ' = ' + (unsafe if sensitive else safe)
                                + '\ndef invoke(target):\n    ' + body + '\n' + invocation + '\n')
                        self.assertEqual(self.inspect(code) is None, not sensitive)

    def test_pr103_nine_v2_implicit_and_variadic_receivers(self) -> None:
        for source in ('breakpoint', 'len'):
            for body in (
                'class C:\n    step = SOURCE\n    def run(self):\n        self.step()\nC().run()',
                'class C:\n    step = SOURCE\n    def invoke(self):\n        self.step()\nfn = C().invoke\nfn()',
                'class C:\n    step = SOURCE\n    @staticmethod\n    def invoke(target):\n        target.step()\nC.invoke(C)',
                'class C: step = SOURCE\ndef invoke(*targets):\n    targets[0].step()\ninvoke(C)',
                'class C: step = SOURCE\ndef invoke(**targets):\n    targets["target"].step()\ninvoke(target=C)',
                'class C: step = SOURCE\ninvoke = lambda target: target.step()\ninvoke(C)',
                'class C: step = SOURCE\ndef invoke(target):\n    if False:\n        invoke(target)\n    target.step()\ninvoke(C)',
            ):
                with self.subTest(source=source, body=body):
                    self.assertEqual(self.inspect(body.replace('SOURCE', source) + '\n') is None, source == 'len')
        self.assertIsNone(self.inspect('class C: step = len\nclass D: step = breakpoint\ndef invoke(target):\n    target.step()\ndef other(target):\n    return None\ninvoke(C)\nother(D)\n'))

    def test_pr103_nine_v2_argument_selection(self) -> None:
        for pack, sensitive in (('(C, D)', False), ('(D, C)', True)):
            with self.subTest(pack=pack):
                code = ('class C: step = len\nclass D: step = breakpoint\ndef invoke(*targets):\n    targets[0].step()\npack = ' + pack + '\ninvoke(*pack)\n')
                self.assertEqual(self.inspect(code) is None, not sensitive)
        for body in (
            'def invoke(target, other):\n    target.step()\npack = (C, D)\ninvoke(*pack)',
            'def invoke(**targets):\n    targets["target"].step()\noptions = {"target": C, "other": D}\ninvoke(**options)',
            'def invoke(*, target):\n    target.step()\ninvoke(target=C)',
            'class E:\n    step = len\n    def invoke(self):\n        self.step()\nE.invoke(E())',
        ):
            with self.subTest(body=body):
                self.assertIsNone(self.inspect('class C: step = len\nclass D: step = breakpoint\n' + body + '\n'))

    def test_pr103_nine_v3_variadic_fixed_prefix(self) -> None:
        for source in ('breakpoint', 'len'):
            for invocation in ('pack = (C,)\ninvoke("safe", *pack)',
                               'pack = ("safe", C)\ninvoke(*pack)',
                               'pack = (C,)\nother = pack\npack = other\ninvoke("safe", *pack)'):
                with self.subTest(source=source, invocation=invocation):
                    code = 'class C: step = ' + source + '\ndef invoke(prefix, *targets):\n    targets[0].step()\n' + invocation + '\n'
                    self.assertEqual(self.inspect(code) is None, source == 'len')

    def test_pr103_nine_v3_callee_value_provenance(self) -> None:
        for container, target in (('[invoke]', 'calls[0]'), ('{"x": invoke}', 'calls["x"]')):
            for source in ('breakpoint', 'len'):
                for operation, argument in (('target.step()', 'C'), ('target()', source)):
                    with self.subTest(container=container, source=source, operation=operation):
                        code = ('class C: step = ' + source + '\ndef invoke(target):\n    ' + operation
                                + '\ncalls = ' + container + '\n' + target + '(' + argument + ')\n')
                        self.assertEqual(self.inspect(code) is None, source == 'len')
            for value in ('os.environ', '"safe"'):
                with self.subTest(container=container, value=value):
                    self.assertEqual(self.inspect('import os, urllib.parse\nenv = ' + value + '\nclass C: encode = urllib.parse.quote\ndef invoke(target):\n    print(target.encode(str(env)))\ncalls = ' + container + '\n' + target + '(C)\n') is None, value == '"safe"')

    def test_pr103_nine_v3_staged_call_edges(self) -> None:
        for source in ('breakpoint', 'len'):
            for body in (
                'class C: step = SOURCE\ndef invoke(target):\n    target.step()\ndef apply(fn, target):\n    fn(target)\napply(invoke, C)',
                'class C: step = SOURCE\ncalls = [lambda target: target.step()]\ncalls[0](C)',
                'class C:\n    step = SOURCE\n    def invoke(self):\n        self.step()\ndef apply(fn):\n    fn()\napply(C().invoke)',
                'class C:\n    step = SOURCE\n    def invoke(self):\n        self.step()\ndef apply(fn, target):\n    fn(target)\napply(C.invoke, C())',
                'class C: step = SOURCE\ndef invoke(target):\n    target.step()\ncalls = [invoke]\nother = calls\ncalls = other\ncalls[0](C)',
                'class C: step = SOURCE\ndef invoke(prefix, *targets):\n    targets[0].step()\nfirst = ("safe",)\nsecond = (C,)\ninvoke(*first, *second)',
            ):
                with self.subTest(source=source, body=body):
                    self.assertEqual(self.inspect(body.replace('SOURCE', source) + '\n') is None, source == 'len')

    def test_pr103_nine_v4_mixed_starred_alternatives(self) -> None:
        for source in ('breakpoint', 'len'):
            for pack in ('(D,) if False else (*other,)', '(*other,) if False else (D,)',
                         '(*other,)', '(D,) if False else (C,)'):
                with self.subTest(source=source, pack=pack):
                    code = ('class C: step = ' + source + '\nclass D: step = len\n'
                            'def invoke(prefix, *targets):\n    targets[0].step()\n'
                            'other = (C,)\npack = ' + pack + '\ninvoke("safe", *pack)\n')
                    self.assertEqual(self.inspect(code) is None, source == 'len')
        self.assertIsNone(self.inspect('class C: step = len\nclass D: step = breakpoint\ndef invoke(prefix, *targets):\n    targets[0].step()\nother = (C, D)\npack = (C, D) if False else (*other,)\ninvoke("safe", *pack)\n'))
        for source in ('breakpoint', 'len'):
            with self.subTest(cyclic_source=source):
                self.assertEqual(self.inspect('class C: step = ' + source + '\nclass D: step = len\ndef invoke(prefix, *targets):\n    targets[0].step()\nother = (*other, C)\npack = (D,) if False else (*other,)\ninvoke("safe", *pack)\n') is None, source == 'len')

    def test_pr103_nine_v4_supplied_descriptor_receivers(self) -> None:
        for source in ('breakpoint', 'len'):
            for body in (
                'def apply(cls):\n    cls.invoke(None, C)\napply(E)',
                'def apply(cls):\n    alias = cls.invoke\n    alias(None, C)\napply(E)',
                'def apply(cls):\n    cls.invoke(None, C)\nalias = E\napply(alias)',
                'def apply(obj):\n    obj.invoke(C)\napply(E())',
                'E.invoke(None, C)',
            ):
                with self.subTest(source=source, body=body):
                    code = ('class C: step = ' + source + '\nclass E:\n'
                            '    def invoke(self, target):\n        target.step()\n' + body + '\n')
                    self.assertEqual(self.inspect(code) is None, source == 'len')

    def test_pr103_nine_v4_descriptor_precision(self) -> None:
        prefix = ('class C: step = len\nclass D: step = breakpoint\nclass E:\n'
                  '    def invoke(self, target, *rest):\n        target.step()\n')
        for body in (
            'def apply(cls):\n    cls.invoke(D, C)\napply(E)',
            'def apply(obj):\n    obj.invoke(C, D)\napply(E())',
            'def apply(cls):\n    cls.invoke(D, C)\nvalues = [E]\napply(values[0])',
            'def apply(obj):\n    obj.invoke(C, D)\nvalues = [E()]\napply(values[0])',
            'def apply(value):\n    value.invoke(C, C)\napply(E if False else E())',
        ):
            with self.subTest(safe_body=body):
                self.assertIsNone(self.inspect(prefix + body + '\n'))
        for args in ('None, D', 'D, None'):
            with self.subTest(ambiguous_args=args):
                self.assertIsNotNone(self.inspect(prefix + 'def apply(value):\n    value.invoke(' + args + ')\napply(E if False else E())\n'))

    def test_pr103_nine_v5_nested_starred_rows(self) -> None:
        for source in ('breakpoint', 'len'):
            for row in ('(D,) if False else (*other,)', '(*other,) if False else (D,)',
                        '(*other,)', '(C,)', '(D,) if False else (C,)'):
                for operation in ('for (target,) in rows:\n    target.step()',
                                  'values = [target.step() for (target,) in rows]'):
                    with self.subTest(source=source, row=row, operation=operation):
                        code = ('class C: step = ' + source + '\nclass D: step = len\nother = (C,)\n'
                                'rows = [' + row + ']\n' + operation + '\n')
                        self.assertEqual(self.inspect(code) is None, source == 'len')

    def test_pr103_nine_v5_row_projection_controls(self) -> None:
        prefix = 'class C: step = len\nclass D: step = breakpoint\nother = (C, D)\n'
        for body in (
            'rows = [(*other,)]\nfor (target, unused) in rows:\n    target.step()',
            'rows = [(C, D) if False else (*other,)]\nvalues = [target.step() for (target, unused) in rows]',
            'rows = [(*other,)]\nfor (target, *rest) in rows:\n    target.step()',
            'rows = [((*other,),)]\nfor ((target, unused),) in rows:\n    target.step()',
        ):
            with self.subTest(safe_body=body):
                self.assertIsNone(self.inspect(prefix + body + '\n'))
        self.assertIsNotNone(self.inspect(prefix + 'rows = [(*other,)]\nfor (*rest, target) in rows:\n    target.step()\n'))
        for source in ('breakpoint', 'len'):
            with self.subTest(cyclic_source=source):
                self.assertEqual(self.inspect('class C: step = ' + source + '\nother = (*other, C)\nrows = [(*other,)]\nfor (target,) in rows:\n    target.step()\n') is None, source == 'len')

    def test_pr103_named_expression_path_assignment_targets(self) -> None:
        for path in ('docs/EXECUTION.md', '/private/issue79-fixture'):
            for binding in ('root = Path(PATH)\n(marker := 0)',
                            '(root := Path(PATH))',
                            'root: object = Path(PATH)\n(marker := 0)',
                            'other = root = Path(PATH)\n(marker := 0)'):
                with self.subTest(path=path, binding=binding):
                    body = 'from pathlib import Path\n' + binding.replace('PATH', repr(path)) + '\nroot.read_text()\n'
                    self.assertEqual(self.inspect(body) is None, path == 'docs/EXECUTION.md')

    def test_pr103_reflective_helper_return_policy_stays_conservative(self) -> None:
        for returned in ('f"line {getattr(node, \'lineno\', 0)}"',
                         'f"line {getattr(node, \'lineno\', getattr(node, \'offset\', 0))}"'):
            with self.subTest(conservative_return=returned):
                self.assertIsNotNone(self.inspect('def describe(node):\n    return ' + returned + '\ntext = describe(None)\n'))
        for returned in ('getattr(node, name)',
                         'f"line {getattr(node, \'lineno\', 0)}" if False else getattr(node, name)',
                         '(getattr(node, name),)[0]',
                         '[f"line {getattr(node, \'lineno\', 0)}", getattr(node, name)]'):
            with self.subTest(unsafe_return=returned):
                self.assertIsNotNone(self.inspect('def extract(node, name):\n    return ' + returned + '\nmember = extract(None, "move_into")\nmember("fixture")\n'))
        self.assertIsNotNone(self.inspect('import os\ndef describe(node):\n    return f"{getattr(node, \'value\', os.environ)}"\ntext = describe(None)\nprint(text)\n'))

    def test_pr103_iterator_diagnostic_metadata_is_structural(self) -> None:
        scanner_tree = ast.parse(_scanner_module_source(PACKET_TEXT))
        helper = next(node for node in scanner_tree.body if isinstance(node, ast.FunctionDef)
                      and node.name == 'python_sensitive_read_violation')
        diagnostic = next(node for node in ast.walk(helper) if isinstance(node, ast.Return)
                          and isinstance(node.value, ast.JoinedStr)
                          and any(isinstance(part, ast.Constant) and isinstance(part.value, str)
                                  and part.value.startswith('Python custom iterator may expose sensitive protocol values ')
                                  for part in node.value.values))
        body = 'import ast\ndef describe(node):\n    ' + ast.unparse(diagnostic).replace('\n', '\n    ') + '\ntext = describe(None)\n'
        self.assertIsNone(self.inspect(body))
        # All candidates come from ast.parse/ast.walk, never generated or
        # executed specimen nodes. A comprehension's iter is an expression.
        for source in ('for value in values:\n    pass\n',
                       'async def collect():\n    async for value in values:\n        pass\n',
                       'rows = [value for value in values]\n',
                       'rows = {value for value in values}\n',
                       'rows = (value for value in values)\n',
                       'rows = {value: value for value in values}\n'):
            for node in ast.walk(ast.parse(source)):
                if isinstance(node, (ast.For, ast.AsyncFor, ast.comprehension)):
                    location = node.iter if isinstance(node, ast.comprehension) else node
                    self.assertIsInstance(location.lineno, int)
                    self.assertGreater(location.lineno, 0)
        for operation, line in (('for item in K():\n    pass\n', 4),
                                ('async def collect():\n    async for item in K():\n        pass\n', 5),
                                ('rows = [item\n    for item in\n    K()]\n', 6)):
            for source in ('os.environ.items()', '[("safe", "value")]'):
                with self.subTest(operation=operation, source=source):
                    result = self.inspect('import os\nclass K:\n    __iter__ = lambda self: iter(' + source + ')\n' + operation)
                    if source.startswith('['):
                        self.assertIsNone(result)
                    else:
                        self.assertEqual(result, 'Python custom iterator may expose sensitive protocol values on line ' + str(line))

    def test_pr103_four_factory_encoder_provenance(self) -> None:
        for setup, factory in (
            ('import functools', 'functools.partial'),
            ('from functools import partial as bind', 'bind'),
            ('import functools\nbind = functools.partial', 'bind'),
        ):
            for value in ('os.environ', '"safe"'):
                with self.subTest(factory=factory, setup=setup, value=value):
                    result = self.inspect(
                        'import os, urllib.parse\n' + setup
                        + '\nencode = ' + factory + '(urllib.parse.quote)\nprint(encode(str('
                        + value + ')))\n')
                    self.assertIsNotNone(result)
                    self.assertIn("not in the reviewed non-network allowlist", result)

    def test_pr103_four_setter_protocol_name_aliases(self) -> None:
        for setup, name in (
            ('name = "__str__"', 'name'),
            ('first = "__str__"\nname = first', 'name'),
            ('name = "__" + "str__"', 'name'),
            ('', 'unknown_name'),
        ):
            for value in ('str(os.environ)', '"safe"'):
                with self.subTest(setup=setup, name=name, value=value):
                    result = self.inspect('import os\ndef leak(self):\n    return ' + value
                        + '\nclass K: pass\n' + setup + '\nsetattr(K, ' + name + ', leak)\nprint(K())\n')
                    self.assertEqual(result is None, value == '"safe"' and name != 'unknown_name')
        self.assertIsNone(self.inspect('class K: pass\nname = "label"\nsetattr(K, name, "safe")\nprint(K())\n'))
        for value in ('str(os.environ)', '"safe"'):
            with self.subTest(shadowed_name=True, value=value):
                self.assertEqual(self.inspect('import os\nname = "label"\ndef leak(self):\n    return '
                    + value + '\nclass K: pass\ndef install(name):\n    setattr(K, name, leak)\n'
                    + 'install("__str__")\nprint(K())\n') is None, value == '"safe"')
                self.assertIsNotNone(self.inspect('import os\nname = "label"\ndef leak(self):\n    return '
                    + value + '\nclass K: pass\ndef install(name):\n    setattr(K, name + "", leak)\n'
                    + 'install("__str__")\nprint(K())\n'))

    def test_pr103_four_async_iterator_protocols(self) -> None:
        for protocol, statement in (('__aiter__', 'yield'), ('__anext__', 'return')):
            for sink in ('async for item in K():\n        print(item)',
                         'print([item async for item in K()])',
                         'print({item async for item in K()})',
                         'print({"key": item async for item in K()})',
                         'print((item async for item in K()))'):
                for value in ('os.environ', '"safe"'):
                    with self.subTest(protocol=protocol, sink=sink, value=value):
                        other = '    def __aiter__(self):\n        return self\n' if protocol == '__anext__' else ''
                        self.assertEqual(self.inspect('import os\nclass K:\n' + other
                            + '    async def ' + protocol + '(self):\n        ' + statement + ' ' + value
                            + '\nasync def collect():\n    ' + sink + '\n') is None, value == '"safe"')
        self.assertIsNone(self.inspect('import os\nclass K:\n    async def __anext__(self):\n        return os.environ\n    def __iter__(self):\n        return iter(["safe"])\nprint([item for item in K()])\n'))
        for installation in ('K.__aiter__ = stream', 'setattr(K, "__aiter__", stream)',
                             'name = "__aiter__"\nsetattr(K, name, stream)'):
            for value in ('os.environ', '"safe"'):
                with self.subTest(installation=installation, value=value):
                    self.assertEqual(self.inspect('import os\nasync def stream(self):\n    yield ' + value
                        + '\nclass K: pass\n' + installation
                        + '\nasync def collect():\n    print([item async for item in K()])\n') is None, value == '"safe"')

    def test_pr103_two_callback_protocol_names(self) -> None:
        for callback in ('list(map(install, ["__str__"]))',
                         '[install(part) for part in ["__str__"]]'):
            for expression in ('name', 'alias'):
                for value in ('str(os.environ)', '"safe"'):
                    with self.subTest(callback=callback, expression=expression, value=value):
                        result = self.inspect('import os\nname = "label"\nalias = "label"\n'
                            + 'def leak(self):\n    return ' + value + '\nclass K: pass\n'
                            + 'def install(name):\n    alias = name\n    setattr(K, ' + expression
                            + ', leak)\n' + callback + '\nprint(K())\n')
                        if callback.startswith('list(map'):
                            self.assertIsNotNone(result)
                            self.assertIn('unresolved member name', result)
                        else:
                            self.assertEqual(result is None, value == '"safe"')
        self.assertIsNone(self.inspect('class K: pass\nfirst = "label"\nname = first\n'
            + 'setattr(K, name, "safe")\nprint(K())\n'))

    def test_pr103_two_returned_async_iterators(self) -> None:
        for returned in ('L()', 'iterator', 'alias', 'L() if flag else self'):
            for installation in ('direct', 'K.__aiter__ = start', 'setattr(K, "__aiter__", start)'):
                for value in ('os.environ', '"safe"'):
                    with self.subTest(returned=returned, installation=installation, value=value):
                        start = 'def start(self):\n    return ' + returned + '\n'
                        definition = ('class K:\n    def __aiter__(self):\n        return ' + returned
                            + '\n' if installation == 'direct' else 'class K: pass\n' + start + installation + '\n')
                        result = self.inspect('import os\nclass L:\n    async def __anext__(self):\n'
                            + '        return ' + value + '\niterator = L()\nalias = iterator\n'
                            + definition + 'async def collect():\n    print([item async for item in K()])\n'
                            + 'collect().send(None)\n')
                        self.assertEqual(result is None, value == '"safe"')
        for returned in ('unknown', 'alias'):
            self.assertIsNotNone(self.inspect('alias = alias\nclass K:\n'
                + '    def __aiter__(self):\n        return ' + returned
                + '\nasync def collect():\n    print([item async for item in K()])\n'))
        self.assertIsNone(self.inspect('class K:\n    def __aiter__(self):\n        return L()\n'
            + 'class L:\n    def __aiter__(self):\n        return K()\n'
            + '    async def __anext__(self):\n        return "safe"\n'
            + 'async def collect():\n    print([item async for item in K()])\n'))
        for setter in ('L.__anext__ = next_item', 'setattr(L, "__anext__", next_item)'):
            for value in ('os.environ', '"safe"'):
                for sink in ('async for item in K():\n        print(item)',
                             'print({item async for item in K()})',
                             'print({"key": item async for item in K()})',
                             'print((item async for item in K()))'):
                    with self.subTest(setter=setter, value=value, sink=sink):
                        self.assertEqual(self.inspect('import os\nclass L: pass\n'
                            + 'async def next_item(self):\n    return ' + value + '\n' + setter
                            + '\nclass K: pass\nK.__aiter__ = lambda self: L()\n'
                            + 'async def collect():\n    ' + sink + '\n') is None, value == '"safe"')

    def test_pr103_two_async_return_binding_alternatives(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    + '        print(item)\n        break\ncollect().send(None)\n')
        for value in ('os.environ', '"safe"'):
            for definition in (
                'class K:\n    def __aiter__(self):\n        self = L()\n        return self\n',
                'class K:\n    def __aiter__(receiver):\n        receiver = L()\n        return receiver\n',
                'class K: pass\ndef start(receiver):\n    receiver = L()\n    return receiver\nK.__aiter__ = start\n',
                'class K:\n    @staticmethod\n    def __aiter__(receiver=L()):\n        return receiver\n',
            ):
                with self.subTest(value=value, definition=definition):
                    self.assertEqual(self.inspect('import os\nclass L:\n'
                        + '    async def __anext__(self):\n        return ' + value + '\n'
                        + definition + consumer) is None, value == '"safe"')
        self.assertIsNone(self.inspect('class K:\n    def __aiter__(receiver):\n'
            + '        return receiver\n    async def __anext__(receiver):\n'
            + '        return "safe"\n' + consumer))
        for returned in ('L()', 'L() if flag else unknown', 'L() if flag else M()',
                         '(L if flag else unknown)()', '(L if flag else M)()'):
            for value in ('os.environ', '"safe"'):
                with self.subTest(returned=returned, value=value):
                    self.assertEqual(self.inspect('import os\nclass L:\n'
                        + '    async def __anext__(self):\n        return ' + value + '\n'
                        + 'class M:\n    async def __anext__(self):\n        return "safe"\n'
                        + 'def choose():\n    return ' + returned + '\nclass K:\n'
                        + '    def __aiter__(self):\n        return choose()\n' + consumer) is None,
                        value == '"safe"' and 'unknown' not in returned and not returned.startswith('('))

    def test_pr103_two_protocol_callable_proofs(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    + '        print(item)\n        break\ncollect().send(None)\n')
        for value in ('os.environ', '"safe"'):
            for method in ('__aiter__', '__anext__'):
                for implementation in ('staticmethod(start)', 'unknown'):
                    with self.subTest(value=value, method=method, implementation=implementation):
                        result = self.inspect('import os\nclass L:\n'
                            + '    async def __anext__(self):\n        return ' + value + '\n'
                            + 'def start(receiver=L()):\n    return receiver\nclass K: pass\n'
                            + 'K.' + method + ' = ' + implementation + '\n' + consumer)
                        self.assertIsNotNone(result)
        for setup in ('factory = L if flag else unknown', 'factory = L\nfactory = unknown',
                      'factory = choose if flag else unknown', 'factory = choose\nfactory = unknown',
                      'factory = L', 'factory = choose', 'factory = L if flag else M',
                      'factory = L if flag else choose'):
            with self.subTest(setup=setup):
                result = self.inspect('class L:\n    async def __anext__(self):\n        return "safe"\n'
                    + 'class M:\n    async def __anext__(self):\n        return "safe"\n'
                    + 'def choose():\n    return L()\n' + setup + '\nclass K:\n'
                    + '    def __aiter__(self):\n        return factory()\n' + consumer)
                self.assertEqual(result is None, 'unknown' not in setup)

    def test_pr103_two_assigned_protocol_receiver_proof(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    + '        print(item)\n        break\ncollect().send(None)\n')
        for value in ('os.environ', '"safe"'):
            for method in ('__aiter__', '__anext__'):
                for setup, implementation in (
                    ('', 'L().start'), ('bound = L().start\nalias = bound\n', 'alias'),
                    ('def implementation():\n    return L().start\n', 'implementation()'),
                ):
                    with self.subTest(value=value, method=method, setup=setup):
                        start = ('    def start(self):\n        return self\n'
                            + '    async def __anext__(self):\n        return ' + value + '\n'
                            if method == '__aiter__' else
                            '    async def start(self):\n        return ' + value + '\n')
                        other = '' if method == '__aiter__' else '    def __aiter__(self):\n        return self\n'
                        result = self.inspect('import os\nclass L:\n' + start
                            + 'class K:\n' + (other or '    pass\n') + setup
                            + 'K.' + method + ' = ' + implementation + '\n' + consumer)
                        self.assertIsNotNone(result)
        self.assertIsNone(self.inspect('def start(receiver):\n    return receiver\n'
            + 'first = start\nalias = first\nclass K:\n'
            + '    async def __anext__(self):\n        return "safe"\n'
            + 'K.__aiter__ = alias\n' + consumer))

    def test_pr103_three_default_callback_protocol_names(self) -> None:
        for expression in ('name', 'alias'):
            for value in ('str(os.environ)', '"safe"'):
                for calls in ('list(map(install, ["__str__"]))',
                              'install()\nlist(map(install, ["__str__"]))'):
                    with self.subTest(expression=expression, value=value, calls=calls):
                        result = self.inspect('import os\nclass K: pass\n'
                            + 'def leak(self):\n    return ' + value + '\n'
                            + 'def install(name="label"):\n    alias = name\n'
                            + '    setattr(K, ' + expression + ', leak)\n'
                            + calls + '\nprint(K())\n')
                        self.assertIsNotNone(result)
                        self.assertIn('unresolved member name', result)
        for calls in ('install()', 'install("__str__")',
                      'direct = install\ndirect("__str__")'):
            for value in ('str(os.environ)', '"safe"'):
                with self.subTest(calls=calls, value=value):
                    self.assertEqual(self.inspect('import os\nclass K: pass\n'
                        + 'def leak(self):\n    return ' + value + '\n'
                        + 'def install(name="label"):\n    alias = name\n'
                        + '    setattr(K, alias, leak)\n' + calls + '\nprint(K())\n') is None,
                        calls == 'install()' or value == '"safe"')
        self.assertIsNone(self.inspect('class K: pass\nname="label"\nalias=name\n'
            + 'setattr(K, alias, "safe")\nprint(K())\n'))

    def test_pr103_three_returned_anext_awaitables(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    + '        print(item)\n        break\ncollect().send(None)\n')
        for value in ('os.environ', '"safe"'):
            for returned in ('Awaitable()', 'result', 'alias'):
                for installation in ('direct', 'L.__anext__ = advance',
                                     'setattr(L, "__anext__", advance)'):
                    with self.subTest(value=value, returned=returned, installation=installation):
                        method = ('class L:\n    def __anext__(self):\n        return ' + returned + '\n'
                            if installation == 'direct' else
                            'class L: pass\ndef advance(self):\n    return ' + returned + '\n'
                            + installation + '\n')
                        self.assertEqual(self.inspect('import os\nclass Awaitable:\n'
                            + '    def __await__(self):\n        if False:\n            yield None\n'
                            + '        return ' + value + '\nresult = Awaitable()\nalias = result\n'
                            + method + 'class K:\n    def __aiter__(self):\n        return L()\n'
                            + consumer) is None, value == '"safe"')
        for returned in ('unknown', 'Awaitable() if flag else unknown'):
            with self.subTest(returned=returned):
                self.assertIsNotNone(self.inspect('class Awaitable:\n'
                    + '    def __await__(self):\n        if False:\n            yield None\n'
                    + '        return "safe"\nclass L:\n    def __anext__(self):\n'
                    + '        return ' + returned + '\nclass K:\n'
                    + '    def __aiter__(self):\n        return L()\n' + consumer))
        self.assertIsNone(self.inspect('class K:\n    def __aiter__(self):\n'
            + '        return self\n    async def __anext__(self):\n        return "safe"\n' + consumer))

    def test_pr103_three_async_receiver_class_mutation(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    + '        print(item)\n        break\ncollect().send(None)\n')
        for value in ('os.environ', '"safe"'):
            for mutation in ('object.__setattr__(self, "__class__", L)',
                             'self.__class__ = L', 'setattr(self, "__class__", L)',
                             'alias = self\n        object.__setattr__(alias, "__class__", L)',
                             'alias = self\n        alias.__class__ = L',
                             'alias = self\n        setattr(alias, "__class__", L)'):
                with self.subTest(value=value, mutation=mutation):
                    self.assertIsNotNone(self.inspect('import os\nclass L:\n'
                        + '    async def __anext__(self):\n        return ' + value + '\n'
                        + 'class K:\n    def __aiter__(self):\n        ' + mutation
                        + '\n        return self\n' + consumer))
        self.assertIsNone(self.inspect('class K:\n    def __aiter__(self):\n'
            + '        self.label = "safe"\n        return self\n'
            + '    async def __anext__(self):\n        return "safe"\n' + consumer))

    def test_pr103_three_same_class_await_protocol_cycle(self) -> None:
        for value in ('os.environ', '"safe"'):
            with self.subTest(value=value):
                self.assertEqual(self.inspect('import os\nclass L:\n'
                    + '    def __anext__(self):\n        return L()\n'
                    + '    def __await__(self):\n        if False:\n            yield None\n'
                    + '        return ' + value + '\nclass K:\n'
                    + '    def __aiter__(self):\n        return L()\n'
                    + 'async def collect():\n    async for item in K():\n'
                    + '        print(item)\n        break\ncollect().send(None)\n') is None,
                    value == '"safe"')

    def test_pr103_three_helper_receiver_class_mutation(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    + '        print(item)\n        break\ncollect().send(None)\n')
        for value in ('os.environ', '"safe"'):
            for call in ('mutate(self)', 'alias(self)', 'mutate(obj=self)',
                         'receiver = self\n        alias(obj=receiver)'):
                with self.subTest(value=value, call=call):
                    self.assertIsNotNone(self.inspect('import os\nclass L:\n'
                        + '    async def __anext__(self):\n        return ' + value + '\n'
                        + 'def mutate(obj):\n    object.__setattr__(obj, "__class__", L)\n'
                        + 'alias = mutate\nclass K:\n    def __aiter__(self):\n        '
                        + call + '\n        return self\n' + consumer))
        for helper in ('def helper(obj):\n    return None\n',
                       'def helper(obj):\n    helper(obj)\n'):
            with self.subTest(helper=helper):
                self.assertIsNotNone(self.inspect(helper + 'class K:\n'
                    + '    def __aiter__(self):\n        helper(self)\n        return self\n'
                    + '    async def __anext__(self):\n        return "safe"\n' + consumer))
        self.assertIsNone(self.inspect('class K:\n    def __aiter__(self):\n'
            + '        setattr(self, "label", "safe")\n        return self\n'
            + '    async def __anext__(self):\n        return "safe"\n' + consumer))

    def test_pr103_three_receiver_setter_target_exhaustiveness(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    + '        print(item)\n        break\ncollect().send(None)\n')
        for value in ('os.environ', '"safe"'):
            for flag in ('', 'flag = False\n'):
                for alternative in ('mutate', 'unknown'):
                    with self.subTest(value=value, flag=flag, alternative=alternative):
                        self.assertIsNotNone(self.inspect('import os\nclass L:\n'
                            + '    async def __anext__(self):\n        return ' + value + '\n'
                            + 'def mutate(obj, name, val):\n'
                            + '    object.__setattr__(obj, "__class__", L)\n' + flag
                            + 'setter = setattr if flag else ' + alternative + '\n'
                            + 'class K:\n    def __aiter__(self):\n'
                            + '        setter(self, "label", "x")\n        return self\n' + consumer))
        for setter in ('setattr', 'object.__setattr__',
                       'setattr if flag else object.__setattr__'):
            with self.subTest(setter=setter):
                self.assertIsNone(self.inspect('flag = False\nsetter = ' + setter
                    + '\nclass K:\n    def __aiter__(self):\n'
                    + '        setter(self, "label", "safe")\n        return self\n'
                    + '    async def __anext__(self):\n        return "safe"\n' + consumer))
        self.assertIsNotNone(self.inspect('import os\nclass L:\n'
            + '    async def __anext__(self):\n        return os.environ\n'
            + 'class Replacement:\n    @staticmethod\n'
            + '    def __setattr__(obj, name, val):\n'
            + '        obj.__class__ = L\nobject = Replacement\n'
            + 'class K:\n    def __aiter__(self):\n'
            + '        object.__setattr__(self, "label", "x")\n        return self\n' + consumer))

    def test_pr103_three_defaulted_setter_formal_origins(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    + '        print(item)\n        break\ncollect().send(None)\n')
        for value in ('os.environ', '"safe"'):
            for setter in ('setter', 'alias'):
                for calls in ('list(map(install, [mutate]))',
                              'install()\nlist(map(install, [mutate]))',
                              'install(mutate)', 'install()', 'install(setattr)'):
                    with self.subTest(value=value, setter=setter, calls=calls):
                        result = self.inspect('import os\nclass L:\n'
                            + '    async def __anext__(self):\n        return ' + value + '\n'
                            + 'def mutate(obj, name, val):\n'
                            + '    object.__setattr__(obj, "__class__", L)\nclass K:\n'
                            + '    async def __anext__(self):\n        return "safe"\n'
                            + 'def install(setter=setattr):\n    alias = setter\n'
                            + '    def start(self):\n        ' + setter
                            + '(self, "label", "x")\n        return self\n'
                            + '    K.__aiter__ = start\n' + calls + '\n' + consumer)
                        self.assertEqual(result is None, calls in ('install()', 'install(setattr)'))

    def test_pr103_v13_receiver_assignment_escapes(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    '        print(item)\n        break\ncollect().send(None)\n')
        for value in ('os.environ', '"safe"'):
            prefix = ('import os\nclass L:\n    async def __anext__(self):\n'
                      '        return ' + value + '\n')
            for setup, body in (
                ('saved = None\ndef mutate():\n    object.__setattr__(saved, "__class__", L)\n',
                 'global saved\n        saved = self\n        mutate()'),
                ('holder = [None]\ndef mutate():\n    object.__setattr__(holder[0], "__class__", L)\n',
                 'holder[0] = self\n        mutate()'),
                ('class Holder: pass\nholder = Holder()\ndef mutate():\n'
                 '    object.__setattr__(holder.saved, "__class__", L)\n',
                 'holder.saved = self\n        mutate()'),
                ('', 'saved = self\n        def mutate():\n'
                 '            object.__setattr__(saved, "__class__", L)\n        mutate()'),
                ('', 'box = [self]\n        def mutate():\n'
                 '            object.__setattr__(box[0], "__class__", L)\n        mutate()'),
            ):
                with self.subTest(value=value, body=body):
                    self.assertIsNotNone(self.inspect(prefix + setup + 'class K:\n'
                        '    def __aiter__(self):\n        ' + body + '\n        return self\n'
                        '    async def __anext__(self):\n        return "safe"\n' + consumer))
            # Complete nonlocal scope; the module helper cannot see a method local.
            with self.subTest(value=value, storage='nonlocal'):
                self.assertIsNotNone(self.inspect(prefix + 'class K:\n'
                    '    async def __anext__(self):\n        return "safe"\n'
                    'def install():\n    saved = None\n'
                    '    def mutate():\n        object.__setattr__(saved, "__class__", L)\n'
                    '    def start(self):\n        nonlocal saved\n'
                    '        saved = self\n        mutate()\n        return self\n'
                    '    K.__aiter__ = start\ninstall()\n' + consumer))
        for body in ('alias = self\n        other = alias\n        other.label = "safe"',
                     'self.label = "safe"'):
            self.assertIsNone(self.inspect('class K:\n    def __aiter__(self):\n        '
                + body + '\n        return self\n    async def __anext__(self):\n'
                '        return "safe"\n' + consumer))
        # Harmless storage and unknown closure effects remain unsupported.
        for body in ('box = [self]', 'saved = self\n        def helper():\n'
                     '            return saved\n        helper()'):
            with self.subTest(unsupported=body):
                self.assertIsNotNone(self.inspect('class K:\n    def __aiter__(self):\n        '
                    + body + '\n        return self\n    async def __anext__(self):\n'
                    '        return "safe"\n' + consumer))

    def test_pr103_v14_augmented_receiver_escape(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    '        print(item)\n        break\ncollect().send(None)\n')
        # Complete finite report and harmless pair; both remain AST text.
        for value in ('os.environ', '"safe"'):
            with self.subTest(value=value):
                self.assertIsNotNone(self.inspect('import os\nholder = []\n'
                    'def mutate():\n    object.__setattr__(holder[0], "__class__", L)\n'
                    'class L:\n    async def __anext__(self):\n        return ' + value + '\n'
                    'class K:\n    def __aiter__(self):\n        global holder\n'
                    '        holder += [self]\n        mutate()\n        return self\n'
                    '    async def __anext__(self):\n        return "safe"\n' + consumer))

    def test_pr103_v14_augmented_receiver_policy_controls(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    '        print(item)\n        break\ncollect().send(None)\n')
        # A local Name target and receiver-valued Name RHS must not inherit
        # the supported simple assignment alias exception. These finite
        # harmless mutations are conservatively unsupported, not leak proofs.
        for rhs in ('self', 'receiver'):
            with self.subTest(rhs=rhs):
                self.assertIsNotNone(self.inspect('class Holder:\n'
                    '    def __iadd__(self, value):\n        return self\n'
                    'class K:\n    def __aiter__(self):\n        receiver = self\n'
                    '        holder = Holder()\n        holder += ' + rhs + '\n'
                    '        return self\n    async def __anext__(self):\n'
                    '        return "safe"\n' + consumer))
        self.assertIsNone(self.inspect('class K:\n    def __aiter__(self):\n'
            '        receiver = self\n        count = 0\n        count += 1\n'
            '        labels = []\n        labels += ["safe"]\n'
            '        receiver.label = count\n        return self\n'
            '    async def __anext__(self):\n        return "safe"\n' + consumer))

    def test_pr103_v15_augmented_receiver_target(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    '        print(item)\n        break\ncollect().send(None)\n')
        for value in ('os.environ', '"safe"'):
            for body in ('self += 1', 'alias = self\n        alias += 1',
                         'alias = self\n        other = alias\n        other += 1'):
                with self.subTest(value=value, body=body):
                    # Finite sensitive class replacement and harmless policy pair.
                    self.assertIsNotNone(self.inspect('import os\nclass L:\n'
                        '    async def __anext__(self):\n        return ' + value + '\n'
                        'class K:\n    def __iadd__(self, other):\n'
                        '        object.__setattr__(self, "__class__", L)\n        return self\n'
                        '    def __aiter__(self):\n        ' + body + '\n        return self\n'
                        '    async def __anext__(self):\n        return "safe"\n' + consumer))

    def test_pr103_v15_augmented_target_provenance_controls(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    '        print(item)\n        break\ncollect().send(None)\n')
        # Unknown alternatives do not erase the visible receiver origin. These
        # are conservative policy controls, not claimed finite runtime leaks.
        for body in ('alias = self if flag else unknown\n        alias += 1',
                     'alias = unknown if flag else self\n        other = alias\n        other += 1'):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect('flag = True\nclass K:\n'
                    '    def __iadd__(self, other):\n        return self\n'
                    '    def __aiter__(self):\n        ' + body + '\n        return self\n'
                    '    async def __anext__(self):\n        return "safe"\n' + consumer))
        # An unresolved returned iterator must not acquire receiver identity.
        self.assertIsNotNone(self.inspect('class K:\n    def __aiter__(self):\n'
            '        alias = unknown\n        alias += 1\n        return alias\n'
            '    async def __anext__(self):\n        return "safe"\n' + consumer))

    def test_pr103_v16_with_augmented_receiver(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    '        print(item)\n        break\ncollect().send(None)\n')
        for value in ('os.environ', '"safe"'):
            for body in ('alias += 1', 'other = alias\n            other += 1'):
                with self.subTest(value=value, body=body):
                    # Both context-manager methods are present; the sensitive
                    # source is a complete finite path, never executed here.
                    self.assertIsNotNone(self.inspect('import os\nclass L:\n'
                        '    async def __anext__(self):\n        return ' + value + '\n'
                        'class K:\n    def __enter__(self):\n        return self\n'
                        '    def __exit__(self, kind, value, traceback):\n        return False\n'
                        '    def __iadd__(self, other):\n'
                        '        object.__setattr__(self, "__class__", L)\n        return self\n'
                        '    def __aiter__(self):\n        with self as alias:\n            '
                        + body + '\n        return self\n'
                        '    async def __anext__(self):\n        return "safe"\n' + consumer))

    def test_pr103_v16_augmented_unknown_binding_policy(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    '        print(item)\n        break\ncollect().send(None)\n')
        # A harmless local context manager still has an unsupported as binding.
        # Unknown alternatives and alias hops cannot certify nonreceiver origin.
        for body in ('alias += 1', 'other = alias\n            other += 1',
                     'other = 0 if flag else alias\n            other += 1'):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect('flag = True\nclass Manager:\n'
                    '    def __enter__(self):\n        return 0\n'
                    '    def __exit__(self, kind, value, traceback):\n        return False\n'
                    'class K:\n    def __aiter__(self):\n        with Manager() as alias:\n'
                    '            ' + body + '\n        return self\n'
                    '    async def __anext__(self):\n        return "safe"\n' + consumer))

    def test_pr103_v16_direct_getitem_protocol(self) -> None:
        for value in ('os.environ', '"safe"'):
            for body in ('print(K()[0])', 'obj = K()\nprint(obj[0])',
                         'obj = K()\nalias = obj\nprint(alias[0])',
                         'flag = True\nobj = K() if flag else unknown\nprint(obj[0])'):
                with self.subTest(value=value, body=body):
                    self.assertEqual(self.inspect('import os\nclass K:\n'
                        '    def __getitem__(self, key):\n        return ' + value + '\n'
                        + body + '\n') is None, value == '"safe"')
        self.assertIsNone(self.inspect('class K:\n    def __getitem__(self, key):\n'
            '        return "safe"\nflag = True\nobj = K() if flag else ["literal"]\n'
            'print(obj[0])\nprint(["safe"][0])\nprint({"key": "safe"}["key"])\n'))

    def test_pr103_v17_subscription_display_result(self) -> None:
        for value in ('str(os.environ)', '"safe"'):
            for result in ('L()', 'L() if flag else unknown'):
                with self.subTest(value=value, result=result):
                    # Safe object results remain unsupported policy controls.
                    self.assertIsNotNone(self.inspect('import os\nflag = True\nclass L:\n'
                        '    def __str__(self):\n        return ' + value + '\n'
                        'class K:\n    def __getitem__(self, key):\n        return '
                        + result + '\nprint(K()[0])\n'))

    def test_pr103_v17_subscription_bound_result(self) -> None:
        for value in ('os.environ', '"safe"'):
            for result in ('self.leak()', 'self.leak() if flag else unknown'):
                with self.subTest(value=value, result=result):
                    self.assertIsNotNone(self.inspect('import os\nflag = True\nclass K:\n'
                        '    def leak(self):\n        return ' + value + '\n'
                        '    def __getitem__(self, key):\n        return ' + result
                        + '\nprint(K()[0])\n'))
        self.assertIsNotNone(self.inspect('class K:\n    def __getitem__(self, key):\n'
            '        return unknown\nprint(K()[0])\n'))

    def test_pr103_v17_class_subscription(self) -> None:
        for value in ('os.environ', '"safe"'):
            for body in ('print(K[0])', 'alias = K\nprint(alias[0])',
                         'flag = True\nalias = K if flag else unknown\nprint(alias[0])'):
                with self.subTest(value=value, body=body):
                    self.assertEqual(self.inspect('import os\nclass K:\n'
                        '    def __class_getitem__(cls, key):\n        return ' + value
                        + '\n' + body + '\n') is None, value == '"safe"')
        for result in ('literal', 'literal if flag else "other"', 'safe()', '["safe"]'):
            with self.subTest(safe_result=result):
                self.assertIsNone(self.inspect('literal = "safe"\nflag = True\n'
                    'def safe():\n    return "safe"\nclass K:\n'
                    '    def __getitem__(self, key):\n        return ' + result + '\n'
                    '    def __class_getitem__(cls, key):\n        return ' + result + '\n'
                    'alias = K\nprint(K()[0])\nprint(alias[0])\n'))

    def test_pr103_v18_class_receiver_wrappers(self) -> None:
        for value in ('os.environ', '"safe"'):
            for receiver in ('next(x for x in [K])', 'next(iter([K]))', 'tuple([K])[0]'):
                with self.subTest(value=value, receiver=receiver):
                    self.assertEqual(self.inspect('import os\nclass K:\n'
                        '    def __class_getitem__(cls, key):\n        return ' + value
                        + '\nprint((' + receiver + ')[0])\n') is None, value == '"safe"')

    def test_pr103_v18_generator_subscription_policy(self) -> None:
        for value in ('str(os.environ)', '"safe"'):
            for yielded in ('yield L()', 'yield from [L()]'):
                with self.subTest(value=value, yielded=yielded):
                    self.assertIsNotNone(self.inspect('import os\nclass L:\n'
                        '    def __str__(self):\n        return ' + value + '\n'
                        'class K:\n    def __getitem__(self, key):\n        ' + yielded
                        + '\n        return "safe"\nprint(next(K()[0]))\n'))

    def test_pr103_v18_nested_subscription_policy(self) -> None:
        manager = ('class Manager:\n    def __enter__(self):\n        return self\n'
                   '    def __exit__(self, kind, value, traceback):\n        return False\n')
        for value in ('os.environ', '"safe"'):
            for suite, tail in (('if True:', ''), ('try:', '    finally:\n        pass\n'),
                                ('with Manager():', ''), ('for ignored in [0]:', '')):
                for method, receiver in (('__getitem__', 'K()'), ('__class_getitem__', 'K')):
                    with self.subTest(value=value, suite=suite, method=method):
                        self.assertIsNotNone(self.inspect('import os\n' + manager
                            + 'class K:\n    ' + suite + '\n        def ' + method
                            + '(self, key):\n            return ' + value + '\n' + tail
                            + 'print(' + receiver + '[0])\n'))

    def test_pr103_v18_subscription_callable_state_policy(self) -> None:
        for value in ('os.environ', '"safe"'):
            for signature, mutation in (
                ('self, key, value="safe"', 'K.__getitem__.__defaults__ = (VALUE,)'),
                ('self, key, *, value="safe"', 'K.__getitem__.__kwdefaults__["value"] = VALUE'),
                ('self, key, value="safe"', 'alias = K.__getitem__\nalias.__defaults__ = (VALUE,)'),
                ('self, key, value="safe"', 'setattr(K.__getitem__, "__defaults__", (VALUE,))'),
                ('self, key, value="safe"', 'K.__getitem__.__code__ = replacement.__code__'),
            ):
                with self.subTest(value=value, mutation=mutation):
                    self.assertIsNotNone(self.inspect('import os\ndef replacement(self, key, value="safe"):\n'
                        '    return ' + value + '\nclass K:\n    def __getitem__(' + signature
                        + '):\n        return value\n' + mutation.replace('VALUE', value)
                        + '\nprint(K()[0])\n'))

    def test_pr103_v18_subscription_helper_state_closure(self) -> None:
        # A literal result also relies on every proven local helper remaining
        # unchanged; this uses the same callable-state boundary, not effects.
        for value in ('os.environ', '"safe"'):
            with self.subTest(value=value):
                self.assertIsNotNone(self.inspect('import os\ndef helper(value="safe"):\n'
                    '    return value\nclass K:\n    def __getitem__(self, key):\n'
                    '        return helper()\nhelper.__defaults__ = (' + value + ',)\n'
                    'print(K()[0])\n'))

    def test_pr103_v18_wrapper_alias_proof(self) -> None:
        for value in ('os.environ', '"safe"'):
            for body in ('ctor = tuple\nA = K\nprint((ctor([A])[0])[0])',
                         'ctor = tuple\nother = ctor\nA = K\nprint((other([A])[0])[0])',
                         'advance = next\nprint(advance(x for x in [K])[0])',
                         'acquire = iter\nadvance = next\nprint(advance(acquire([K]))[0])'):
                with self.subTest(value=value, body=body):
                    self.assertEqual(self.inspect('import os\nclass K:\n'
                        '    def __class_getitem__(cls, key):\n        return ' + value
                        + '\n' + body + '\n') is None, value == '"safe"')
            # Every visible wrapper target must be proven, even with a safe
            # selected result; an unknown sibling is a policy refusal.
            with self.subTest(value=value, competing=True):
                self.assertIsNotNone(self.inspect('import os\nflag = True\nclass K:\n'
                    '    def __class_getitem__(cls, key):\n        return ' + value + '\n'
                    'ctor = tuple if flag else unknown\nprint(ctor([K])[0][0])\n'))

    def test_pr103_v19_mapping_value_class_receiver(self) -> None:
        for value in ('os.environ', '"safe"'):
            for body in ('print(dict({"x": K})["x"][0])',
                         'ctor = dict\nA = K\nprint(ctor({"x": A})["x"][0])'):
                with self.subTest(value=value, body=body):
                    self.assertEqual(self.inspect('import os\nclass K:\n'
                        '    def __class_getitem__(cls, key):\n        return ' + value
                        + '\n' + body + '\n') is None, value == '"safe"')

    def test_pr103_v19_computed_callable_state_policy(self) -> None:
        for value in ('os.environ', '"safe"'):
            for target in ('tuple([K.__getitem__])[0]', 'next(iter([K.__getitem__]))'):
                with self.subTest(value=value, target=target):
                    self.assertIsNotNone(self.inspect('import os\n'
                        'def replacement(self, key):\n    return ' + value + '\nclass K:\n'
                        '    def __getitem__(self, key):\n        return "safe"\n'
                        'target = ' + target + '\ntarget.__code__ = replacement.__code__\n'
                        'print(K()[0])\n'))

    def test_pr103_v19_class_protocol_binder_policy(self) -> None:
        for value in ('os.environ', '"safe"'):
            for binder in ('for __class_getitem__ in [leak]:\n        pass',
                           '__class_getitem__, other = (leak, None)',
                           '(__class_getitem__ := leak)'):
                with self.subTest(value=value, binder=binder):
                    self.assertIsNotNone(self.inspect('import os\ndef leak(cls, key):\n'
                        '    return ' + value + '\nclass K:\n    ' + binder + '\nprint(K[0])\n'))

    def test_pr103_v19_mutable_protocol_dependencies_policy(self) -> None:
        for value in ('os.environ', '"safe"'):
            for setup, returned, mutation in (
                ('state = ["safe"]', 'state[0]', 'state[0] = ' + value),
                ('state = {"x": "safe"}', 'state["x"]', 'state["x"] = ' + value),
                ('state = (["safe"],)', 'state[0][0]', 'state[0][0] = ' + value),
            ):
                with self.subTest(value=value, setup=setup):
                    self.assertIsNotNone(self.inspect('import os\ndef factory():\n    ' + setup
                        + '\n    class K:\n        def __getitem__(self, key):\n            return '
                        + returned + '\n    ' + mutation + '\n    return K\n'
                        'K = factory()\nprint(K()[0])\n'))
        # A mutable default with an unclosed write is also unsupported, while
        # the literal argument/default itself is not a sensitive value.
        self.assertIsNotNone(self.inspect('class K:\n'
            '    def __getitem__(self, key, state=["safe"]):\n        return state[0]\n'
            'print(K()[0])\n'))

    def test_pr103_v20_keyword_mapping_receiver(self) -> None:
        for value in ('os.environ', '"safe"'):
            for body in ('print(dict(x=K)["x"][0])',
                         'print(dict(**{"x": K})["x"][0])',
                         'values = {"x": K}\nctor = dict\nprint(ctor(**values)["x"][0])'):
                with self.subTest(value=value, body=body):
                    self.assertEqual(self.inspect('import os\nclass K:\n'
                        '    def __class_getitem__(cls, key):\n        return ' + value
                        + '\n' + body + '\n') is None, value == '"safe"')

    def test_pr103_v20_member_mutable_dependency_policy(self) -> None:
        for value in ('os.environ', '"safe"'):
            for returned in ('Holder.state[0]', 'alias.state[0]'):
                with self.subTest(value=value, returned=returned):
                    self.assertIsNotNone(self.inspect('import os\nclass Holder:\n'
                        '    state = ["safe"]\nalias = Holder\nclass K:\n'
                        '    def __getitem__(self, key):\n        return ' + returned
                        + '\nHolder.state[0] = ' + value + '\nprint(K()[0])\n'))

    def test_pr103_v20_descriptor_construction_hook_policy(self) -> None:
        for value in ('os.environ', '"safe"'):
            for setup in ('class Descriptor:\n    def __set_name__(self, owner, name):\n'
                          '        owner.__class_getitem__ = lambda key: VALUE\n',
                          'def install(self, owner, name):\n'
                          '    owner.__class_getitem__ = lambda key: VALUE\n'
                          'class Descriptor: pass\nDescriptor.__set_name__ = install\n'):
                with self.subTest(value=value, setup=setup):
                    self.assertIsNotNone(self.inspect('import os\n' + setup.replace('VALUE', value)
                        + 'class K:\n    field = Descriptor()\nprint(K[0])\n'))

    def test_pr103_v20_callback_state_target_policy(self) -> None:
        for value in ('os.environ', '"safe"'):
            for invoke in ('list(map(mutate, [K.__getitem__]))',
                           'alias = mutate\nlist(map(alias, [K.__getitem__]))'):
                with self.subTest(value=value, invoke=invoke):
                    self.assertIsNotNone(self.inspect('import os\n'
                        'def replacement(self, key):\n    return ' + value + '\n'
                        'def safe(self, key):\n    return "safe"\n'
                        'def mutate(target=safe):\n    target.__code__ = replacement.__code__\n'
                        'class K:\n    def __getitem__(self, key):\n        return "safe"\n'
                        + invoke + '\nprint(K()[0])\n'))
        self.assertIsNone(self.inspect('def safe(self, key):\n    return "safe"\n'
            'def replacement(self, key):\n    return "safe"\n'
            'def mutate(target=safe):\n    target.__code__ = replacement.__code__\n'
            'class K:\n    def __getitem__(self, key):\n        return "safe"\n'
            'mutate()\nprint(K()[0])\n'))

    def test_pr103_v20_computed_hook_installation_policy(self) -> None:
        for value in ('os.environ', '"safe"'):
            for mutation in ('alias.__set_name__ = install',
                             'setattr(alias, "__set_name__", install)'):
                with self.subTest(value=value, mutation=mutation):
                    self.assertIsNotNone(self.inspect('import os\n'
                        'def install(self, owner, name):\n'
                        '    owner.__class_getitem__ = lambda key: ' + value + '\n'
                        'class Descriptor: pass\nalias = tuple([Descriptor])[0]\n'
                        + mutation + '\nclass K:\n    field = Descriptor()\nprint(K[0])\n'))

    def test_pr103_v21_mutated_wrapper_mapping_policy(self) -> None:
        for value in ('os.environ', '"safe"'):
            for mutation in ('values["x"] = K', 'alias = values\nalias["x"] = K',
                             'values.update(x=K)', 'values |= {"x": K}'):
                with self.subTest(value=value, mutation=mutation):
                    self.assertIsNotNone(self.inspect('import os\nclass K:\n'
                        '    def __class_getitem__(cls, key):\n        return ' + value + '\n'
                        'values = {"x": "safe"}\n' + mutation
                        + '\nprint(dict(**values)["x"][0])\n'))

    def test_pr103_v21_projected_callback_parameter_policy(self) -> None:
        for value in ('os.environ', '"safe"'):
            for body in ('box[0].__code__ = replacement.__code__',
                         'target = box[0]\n    target.__code__ = replacement.__code__',
                         'chosen = (box[0],)\n    chosen[0].__code__ = replacement.__code__'):
                with self.subTest(value=value, body=body):
                    self.assertIsNotNone(self.inspect('import os\n'
                        'def replacement(self, key):\n    return ' + value + '\n'
                        'def safe(self, key):\n    return "safe"\n'
                        'def mutate(box=(safe,)):\n    ' + body + '\nclass K:\n'
                        '    def __getitem__(self, key):\n        return "safe"\n'
                        'list(map(mutate, [(K.__getitem__,)]))\nprint(K()[0])\n'))
        self.assertIsNone(self.inspect('def safe(self, key):\n    return "safe"\n'
            'def replacement(self, key):\n    return "safe"\n'
            'def mutate(box=(safe,)):\n    box[0].__code__ = replacement.__code__\n'
            'class K:\n    def __getitem__(self, key):\n        return "safe"\n'
            'mutate()\nprint(K()[0])\n'))

    def test_pr103_v21_comprehension_member_policy(self) -> None:
        for value in ('os.environ', '"safe"'):
            for initial, returned, mutation in (
                ('[x for x in ["safe"]]', 'Holder.state[0]', 'Holder.state[0] = VALUE'),
                ('{x: "safe" for x in ["x"]}', 'Holder.state["x"]', 'Holder.state["x"] = VALUE'),
                ('{x for x in ["safe"]}', 'next(iter(Holder.state))',
                 'Holder.state.clear()\nHolder.state.add(str(VALUE))'),
            ):
                with self.subTest(value=value, initial=initial):
                    self.assertIsNotNone(self.inspect('import os\nclass Holder:\n    state = '
                        + initial + '\nclass K:\n    def __getitem__(self, key):\n        return '
                        + returned + '\n' + mutation.replace('VALUE', value) + '\nprint(K()[0])\n'))

    def test_pr103_v21_local_class_scope_coherence(self) -> None:
        self.assertIsNone(self.inspect('values = {"x": "safe"}\n'
            'values["x"] = "reviewed"\nprint(dict(**values)["x"])\n'))
        for value in ('os.environ', '"safe"'):
            with self.subTest(local_class_result=value):
                self.assertIsNotNone(self.inspect('import os\nclass K:\n'
                    '    def __class_getitem__(cls, key):\n        return ' + value + '\n'
                    'values = {"x": "safe"}\nvalues["x"] = K\n'
                    'print(dict(**values)["x"][0])\n'))

    def test_pr103_v21_inherited_mutated_mapping_value_output(self) -> None:
        for mutation in ('values["x"] = os.environ',
                         'alias = values\nalias["x"] = os.environ',
                         'values.update(x=os.environ)', 'values |= {"x": os.environ}'):
            for output in ('print(dict(**values)["x"])',
                           'copied = dict(**values)\nprint(copied["x"])',
                           'print(dict(values)["x"])'):
                with self.subTest(mutation=mutation, output=output):
                    self.assertIsNotNone(self.inspect('import os\nvalues = {"x": "safe"}\n'
                        + mutation + '\n' + output + '\n'))

    def test_pr103_v21_mutated_mapping_value_output_controls(self) -> None:
        for body in (
            'values = {"x": "safe"}\nprint(dict(**values)["x"])\n',
            'values = {"x": "safe"}\nvalues["x"] = "reviewed"\nprint(dict(**values)["x"])\n',
            'import os\nvalues = {"x": "safe"}\nvalues["other"] = os.environ\n'
            'print(dict(**values)["x"])\n',
            'import os\nvalues = {"x": "safe"}\nvalues["x"] = os.environ\n'
            'for key in values:\n    {}[key]\n',
        ):
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_pr103_v21_selected_store_scope_collision(self) -> None:
        self.assertIsNone(self.inspect('import os\ndef unrelated():\n'
            '    value = os.environ\n    return value\n'
            'value = "safe"\nvalues = {"x": "safe"}\nvalues["x"] = value\n'
            'print(dict(**values)["x"])\n'))
        for binding in ('value = os.environ',
                        'value = "safe" if flag else os.environ',
                        'secret = os.environ\nvalue = secret'):
            with self.subTest(binding=binding):
                self.assertIsNotNone(self.inspect('import os\n' + binding
                    + '\nvalues = {"x": "safe"}\nvalues["x"] = value\n'
                    'print(dict(**values)["x"])\n'))

    def test_pr103_v22_ordered_mapping_snapshots(self) -> None:
        prefix = 'import os\nm = {"x": "old"}\nm["x"] = os.environ\n'
        for middle in ('m["x"] = "safe"\nc = dict(m)',
                       'alias = m\nalias["x"] = "safe"\nc = dict(**m)',
                       'm.update(x="safe")\nc = dict(m)',
                       'm |= {"x": "safe"}\nc = dict(m)'):
            with self.subTest(middle=middle):
                self.assertIsNone(self.inspect(prefix + middle + '\nprint(c["x"])\n'))
        for middle in ('c = dict(m)\nm["x"] = "safe"',
                       'c = dict(m)\nalias = c\nalias["x"] = os.environ',
                       'alias = m\nm = {"x": "safe"}\nc = dict(alias)',
                       'if flag:\n    m["x"] = "safe"\nc = dict(m)',
                       'for _ in unknown:\n    m["x"] = "safe"\nc = dict(m)',
                       'try:\n    m["x"] = "safe"\nexcept Exception:\n    pass\nc = dict(m)'):
            with self.subTest(middle=middle):
                self.assertIsNotNone(self.inspect(prefix + middle + '\nprint(c["x"])\n'))
        for suffix in (
            'print(dict(m, extra=(m := {"x": "safe"}))["x"])',
            'print(m["x" if (m := {"x": "safe"}) else "x"])',
            'alias = m\nm["x"] = "safe" if (m := {"x": "safe"}) else "safe"\n'
            'print(dict(alias)["x"])',
        ):
            with self.subTest(evaluation_order=suffix):
                self.assertIsNotNone(self.inspect(prefix + suffix + '\n'))

    def test_pr103_v22_finite_mapping_store_domains(self) -> None:
        for domain, safe in (('{"other": os.environ}', True),
                             ('{"x": os.environ}', False),
                             ('{name: os.environ}', False),
                             ('{"other": os.environ, **unknown}', False),
                             ('{"other": os.environ} if flag else unknown', False)):
            body = ('import os\nm = {"x": "safe"}\ndomain = ' + domain
                    + '\nfor key, value in domain.items():\n    m[key] = value\n'
                    + 'print(dict(m)["x"])\n')
            with self.subTest(domain=domain):
                self.assertEqual(self.inspect(body) is None, safe)

    def test_pr103_v22_escape_and_callback_refusals(self) -> None:
        prefix = 'import os\nimport subprocess\nimport sys\nm = {"x": "safe"}\n'
        for middle in (
            'unknown(m)',
            'alias = m\nunknown(alias)\nm["x"] = "safe"',
            'def touch(arg):\n    arg["x"] = os.environ\ntouch(m)',
            'def touch(arg, callback=unknown):\n    callback(arg)\ntouch(m)',
            'def touch(arg):\n    global m\n    m["x"] = os.environ\ntouch(m)',
            'def hook(event, args):\n    unknown(args)\nsys.addaudithook(hook)\n'
            'subprocess.Popen(["printf", "safe"], env=m)',
            'audit = sys.addaudithook\naudit(unknown)\n'
            'subprocess.Popen(["printf", "safe"], env=m)',
            'sp = subprocess\ndef patch():\n    sp.Popen = unknown\npatch()\n'
            'subprocess.Popen(["printf", "safe"], env=m)',
            'sys.settrace(unknown)\nsubprocess.Popen(["printf", "safe"], env=m)',
        ):
            with self.subTest(middle=middle):
                self.assertIsNotNone(self.inspect(prefix + middle + '\nprint(dict(m)["x"])\n'))

    def test_pr103_v22_closed_call_positive_pairs(self) -> None:
        prefix = 'import subprocess\nfrom pathlib import Path\nm = {"x": "safe"}\n'
        for use in (
            'def read(arg):\n    return arg.get("x")\nread(m)',
            'subprocess.Popen(["printf", "safe"], env=m)',
            'subprocess.Popen(["printf", "safe"], cwd=Path(".").resolve(), env=m)',
            'def launch(**kwargs):\n    return subprocess.Popen(["printf", "safe"], **kwargs)\nlaunch(env=m)',
        ):
            with self.subTest(use=use):
                self.assertIsNone(self.inspect(prefix + use + '\nprint(dict(m)["x"])\n'))
        for pin in ('if flag:\n        kwargs["cwd"] = "."',
                    'for _ in unknown:\n        kwargs["cwd"] = "."'):
            with self.subTest(weak_kwargs_pin=pin):
                self.assertIsNotNone(self.inspect(prefix
                    + 'def launch(**kwargs):\n    kwargs["cwd"] = unknown\n    ' + pin
                    + '\n    return subprocess.Popen(["printf", "ok"], **kwargs)\n'
                    'launch(env=m)\nprint(dict(m)["x"])\n'))
    def test_pr103_v22_sensitive_slice_and_dependency_bounds(self) -> None:
        prefix = 'import subprocess\nm = {"x": "safe"}\n'
        for bound, safe in (('len("-")', True), ('unknown', False)):
            with self.subTest(slice_bound=bound):
                body = (prefix
                    + 'def flags(args):\n    values = []\n    for value in args:\n'
                    '        values.append(value[' + bound + ':])\n    return values\n'
                    'argv = flags(["-ok"])\nsubprocess.Popen(argv, env=m)\n'
                    'print(dict(m)["x"])\n')
                # This standalone helper-return argv is outside the earlier
                # command-policy subset. Keep its full refusal explicit while
                # testing the separate selected-map/type proof used by the
                # already recognized canonical dynamic command recipe.
                self.assertIsNotNone(self.inspect(body))
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree)
                    for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](
                    tree, parents) is None, safe)
        aliases = 'p0 = "."\n' + ''.join(f'p{i} = p{i-1}\n' for i in range(1, 41))
        body = prefix + aliases + 'subprocess.Popen(["printf", "ok"], cwd=p40, env=m)\nprint(dict(m)["x"])\n'
        tree = ast.parse(body)
        parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
        self.assertIsNone(self.scanner['python_sensitive_read_violation'](tree, parents))
        for count, safe in ((20, True), (40, False)):
            with self.subTest(local_call_budget=count):
                definitions = 'def f0():\n    value = "."\n    return value\n' + ''.join(
                    f'def f{i}():\n    value = f{i-1}()\n    return value\n' for i in range(1, count))
                body = (prefix + definitions + f'subprocess.Popen(["printf", "ok"], cwd=f{count-1}(), env=m)\n'
                    'print(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)
        for setup, later, safe in (
            ('parts = "ok+more".split("+")\ntag, other = parts\n', '', True),
            ('tag = unknown\n', 'tag = "safe"\n', False),
            ('tag = "safe"\ndef inject():\n    global tag\n    tag = unknown\ninject()\n', '', False),
        ):
            with self.subTest(free_global=setup, later=later):
                body = (prefix + setup + 'def argv():\n    return ["printf", tag]\n'
                    'subprocess.Popen(argv(), env=m)\n' + later + 'print(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)
        for program in (
            'tag = unknown\ndef argv():\n    return ["printf", tag]\n'
            'old = argv()\ntag = "safe"\nsubprocess.Popen(old, env=m)',
            'tag = unknown\ndef argv():\n    return ["printf", tag]\n'
            'old = argv()\ntag = "safe"\nnew = argv()\nsubprocess.Popen(old, env=m)',
            'tag = unknown\ndef argv():\n    return ["printf", tag]\n'
            'old = argv()\ntag = "safe"\nnew = argv()\nsubprocess.Popen(new, env=m)',
            'tag = unknown\ndef argv(value=tag):\n    return ["printf", value]\n'
            'tag = "safe"\nsubprocess.Popen(argv(), env=m)',
            'def outer():\n    tag = "safe"\n    def change():\n'
            '        nonlocal tag\n        tag = unknown\n'
            '    change()\n    return ["printf", tag]\nsubprocess.Popen(outer(), env=m)',
        ):
            with self.subTest(invocation_version=program):
                body = prefix + program + '\nprint(dict(m)["x"])\n'
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertIsNotNone(self.scanner['python_sensitive_read_violation'](tree, parents))

    def test_pr103_v22_domain_and_capture_closure(self) -> None:
        for middle in (
            'domain = {"other": os.environ}\ndomain["x"] = os.environ\n'
            'for key, value in domain.items():\n    m[key] = value',
            'domain = {"other": os.environ}\nalias = domain\nalias["x"] = os.environ\n'
            'for key, value in domain.items():\n    m[key] = value',
            'key = "other"\ndomain = {"x": os.environ}\n'
            'for key, value in domain.items():\n    m[key] = value',
            'def touch():\n    m["x"] = os.environ\ntouch()',
            'holder = m\ndef touch(arg=holder):\n    arg["x"] = os.environ\ntouch()',
        ):
            with self.subTest(middle=middle):
                self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
                    + middle + '\nprint(dict(m)["x"])\n'))
        for source in ('factory()', 'unknown(m)'):
            with self.subTest(source=source):
                self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
                    'm["x"] = os.environ\ndef factory():\n    return m\n'
                    'print(dict(' + source + ')["x"])\n'))

    def test_pr103_v22_primitive_dynamic_key_pins(self) -> None:
        for source, safe in (('list(sys.argv)', True), ('unknown', False),
                             ('list(sys.argv) if flag else unknown', False)):
            with self.subTest(source=source):
                self.assertEqual(self.inspect('import sys\ncommand = ' + source
                    + '\nm = {}\nwhile command:\n'
                    + '    key, value = command.pop(0).split("=", 1)\n'
                    + '    m[key] = value\nm["x"] = "safe"\n'
                    + 'print(dict(m)["x"])\n') is None, safe)
        dynamic_child = ('import sys\nimport subprocess\ncommand = list(sys.argv)\n'
            'm = {}\nwhile command:\n    key, value = command.pop(0).split("=", 1)\n'
            '    m[key] = value\nm["x"] = "safe"\nc = dict(m)\n')
        # Existing child-environment policy rejects an arbitrary copy binding;
        # the new selected-store proof cannot discharge that independent gate.
        self.assertIsNotNone(self.inspect(dynamic_child
            + 'subprocess.Popen(["printf", "safe"], env=c)\nprint(dict(c)["x"])\n'))
        self.assertIsNone(self.inspect(dynamic_child
            + 'subprocess.Popen(["printf", "safe"], env=m)\nprint(dict(m)["x"])\n'))

    def test_pr103_v22_unrelated_sequence_selections(self) -> None:
        for body in ('print(("safe",)[0])\n', 'print(["safe"][0])\n',
                     'values = ["safe"]\nprint(values[0])\n'):
            with self.subTest(body=body):
                self.assertIsNone(self.inspect(body))

    def test_pr103_v22_process_field_reflection(self) -> None:
        prefix = ('import subprocess\nm = {"x": "safe"}\n'
                  'p = subprocess.Popen(["printf", "safe"], env=m)\n')
        for field, safe in (('"stdout"', True),
                            ('"stdout" if flag else "stderr"', True),
                            ('"stdout" if flag else unknown', False),
                            ('unknown', False)):
            for getter in ('getattr', 'reader'):
                with self.subTest(field=field, getter=getter):
                    self.assertEqual(self.inspect(prefix + 'field = ' + field
                        + '\nreader = getattr\nstream = ' + getter
                        + '(p, field, None)\nprint(dict(m)["x"])\n') is None, safe)
        for setup in ('p = unknown\n', 'p = p if flag else unknown\n',
                      'p.stdout = unknown\n'):
            with self.subTest(setup=setup):
                self.assertIsNotNone(self.inspect(prefix + setup
                    + 'stream = getattr(p, "stdout", None)\nprint(dict(m)["x"])\n'))
        self.assertIsNotNone(self.inspect(prefix
            + 'stream = getattr(p, "stdout", None)\nunknown(stream)\nprint(dict(m)["x"])\n'))
        for setup in ('field = unknown\nif flag:\n    field = "stdout"\n',
                      'field = unknown\nfor _ in unknown:\n    field = "stdout"\n'):
            with self.subTest(weak_field_pin=setup):
                self.assertIsNotNone(self.inspect(prefix + setup
                    + 'stream = getattr(p, field, None)\nprint(dict(m)["x"])\n'))
        for fields, before_read, safe in (
            ('("stdin", "stdout", "stderr")', '', True),
            ('("stdout", unknown)', '', False),
            ('("stdout",)', '    name = unknown\n', False),
            ('("stdout",)', '    if flag:\n        name = unknown\n', False),
        ):
            with self.subTest(fields=fields, before_read=before_read):
                self.assertEqual(self.inspect(prefix + 'for name in ' + fields + ':\n'
                    + before_read + '    stream = getattr(p, name, None)\n'
                    'print(dict(m)["x"])\n') is None, safe)

    def test_pr103_v22_concrete_primitive_callback_leaks(self) -> None:
        prefix = 'import os\nimport sys\nimport subprocess\nm = {"x": "safe"}\n'
        for setup, arguments in (
            ('def hook(event, args):\n    if event == "subprocess.Popen":\n'
             '        args[3]["x"] = os.environ\nsys.addaudithook(hook)\n', ''),
            ('def hook(event, args):\n    if event == "subprocess.Popen":\n'
             '        args[3]["x"] = os.environ\n'
             'register = getattr(sys, "addaudithook")\nregister(hook)\n', ''),
            ('class Cwd:\n    def __fspath__(self):\n        m["x"] = os.environ\n'
             '        return "."\n', ', cwd=Cwd()'),
            ('sp = subprocess\ndef leak(*args, **kwargs):\n'
             '    kwargs["env"]["x"] = os.environ\ndef patch():\n'
             '    sp.Popen = leak\npatch()\n', ''),
        ):
            with self.subTest(setup=setup):
                self.assertIsNotNone(self.inspect(prefix + setup
                    + 'subprocess.Popen(["printf", "safe"], env=m' + arguments
                    + ')\nprint(dict(m)["x"])\n'))

    def test_pr103_v22_process_io_dependency_frames(self) -> None:
        prefix = 'import os\nimport subprocess\nimport selectors\nm = {"x": "safe"}\n'
        suffix = '\nprint(dict(m)["x"])\n'
        for use in (
            'def close(process):\n    for name in ("stdin", "stdout", "stderr"):\n'
            '        stream = getattr(process, name, None)\n'
            '        if stream is not None:\n            stream.close()\n'
            'p = subprocess.Popen(["printf", "ok"], env=m, stdout=subprocess.PIPE)\nclose(p)',
            'p = None\nif flag:\n    p = subprocess.Popen(["printf", "ok"], env=m, stdout=subprocess.PIPE)\n'
            'stream = getattr(p, "stdout", None)',
            'p = subprocess.Popen(["printf", "one"], env=m, stdout=subprocess.PIPE)\n'
            'if flag:\n    p = subprocess.Popen(["printf", "two"], env=m, stdout=subprocess.PIPE)\n'
            'stream = getattr(p, "stdout", None)\nif stream is not None:\n    stream.close()',
            'def launch(**kwargs):\n    return subprocess.Popen(["printf", "ok"], stdout=subprocess.PIPE, **kwargs)\n'
            'p = launch(env=m)\nstream = getattr(p, "stdout", None)\n'
            'if stream is not None:\n    stream.close()',
        ):
            with self.subTest(supported=use):
                self.assertIsNone(self.inspect(prefix + use + suffix))
        for use in (
            'def callback():\n    pass\ndef close(process, callback):\n'
            '    callback()\n    stream = getattr(process, "stdout", None)\n'
            'p = subprocess.Popen(["printf", "ok"], env=m, stdout=subprocess.PIPE)\nclose(p, unknown)',
            'def launch(**kwargs):\n    return subprocess.Popen(["printf", "ok"], **kwargs)\n'
            'one = launch(env=m)\ntwo = launch(env=unknown)\nstream = getattr(one, "stdout", None)',
            'p = subprocess.Popen(["printf", "ok"], env=m, stdout=subprocess.PIPE)\n'
            'stream = getattr(p, "stdout", None)\nholder = (stream, m)',
            'p = subprocess.Popen(["printf", "ok"], env=m, stdout=subprocess.PIPE)\n'
            'stream = getattr(p, "stdout", None)\ndef capture(value):\n    m["x"] = os.environ\ncapture(stream)',
            'p = subprocess.Popen(["printf", "ok"], env=m, stdout=subprocess.PIPE)\n'
            'stream = getattr(p, "stdout", None)\nstream.close = unknown',
            'selectors.DefaultSelector.register = unknown\np = subprocess.Popen(["printf", "ok"], env=m)\n'
            'stream = getattr(p, "stdout", None)',
            'p = subprocess.Popen([getattr(p, "stdout", None)], env=m)\n'
            'stream = getattr(p, "stdout", None)',
        ):
            with self.subTest(unsupported=use):
                self.assertIsNotNone(self.inspect(prefix + use + suffix))

    def test_pr103_v22_completed_result_field_values(self) -> None:
        prefix = 'import os\nimport subprocess\nm = {"x": "safe"}\n'
        for field in ('stdout', 'stderr'):
            for value, safe in (('"safe"', True), ('unknown', False), ('os.environ', False), ('[]', False)):
                with self.subTest(field=field, value=value):
                    body = (prefix + 'result = subprocess.CompletedProcess([], 0, ' + field + '=' + value + ')\n'
                        'alias = result\nargv = ["printf", alias.' + field + ']\n'
                        'subprocess.Popen(argv, env=m)\nprint(dict(m)["x"])\n')
                    tree = ast.parse(body)
                    parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                    self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)
        for mutation in ('alias.stdout = unknown', 'alias.stderr = unknown'):
            body = (prefix + 'result = subprocess.CompletedProcess([], 0, stdout="safe", stderr="safe")\n'
                'alias = result\n' + mutation + '\nsubprocess.Popen(["printf", result.stdout], env=m)\n'
                'print(dict(m)["x"])\n')
            tree = ast.parse(body)
            parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
            self.assertIsNotNone(self.scanner['python_sensitive_read_violation'](tree, parents))
        for retained in ('result = subprocess.CompletedProcess([m], 0, stdout="safe")',
                         'def make(value):\n    return subprocess.CompletedProcess([], 0, stdout=value)\n'
                         'result = make("safe")\nother = make(unknown)'):
            body = (prefix + retained + '\nsubprocess.Popen(["printf", result.stdout], env=m)\n'
                'print(dict(m)["x"])\n')
            tree = ast.parse(body)
            parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
            self.assertIsNotNone(self.scanner['python_sensitive_read_violation'](tree, parents))

    def test_pr103_v22_unknown_call_after_mapping_pin(self) -> None:
        prefix = 'import os\nimport builtins\nm = {"x": os.environ}\nm["x"] = "safe"\n'
        for effect in ('unknown()', 'getattr(unknown, "touch")', 'unknown.touch()',
                       'builtins.eval("unknown")'):
            with self.subTest(effect=effect):
                self.assertIsNotNone(self.inspect(prefix + effect + '\nprint(dict(m)["x"])\n'))
        self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
            'def rebind():\n    global m\n    m = {"x": os.environ.get("HOME")}\n'
            'rebind()\nprint(dict(m)["x"])\n'))

    def test_pr103_v22_native_global_writer_types(self) -> None:
        self.assertIsNotNone(self.inspect('import os\nvalue = "safe"\n'
            'def mutate():\n    global value\n    value = os.environ.get("HOME")\n'
            'mutate()\nm = {"x": "old"}\nm["x"] = value\nprint(dict(m)["x"])\n'))
        for value, safe in (('Path("next")', True), ('unknown', False)):
            with self.subTest(global_value=value):
                body = ('import subprocess\nfrom pathlib import Path\nm = {"x": "safe"}\n'
                    'directory = Path(".")\ndef pin():\n    global directory\n    directory = '
                    + value + '\npin()\ndef launch():\n'
                    '    return subprocess.Popen(["printf", "ok"], cwd=directory, env=m)\n'
                    'launch()\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_declared_writer_binding_ownership(self) -> None:
        self.assertIsNone(self.inspect('import os\nvalue = os.environ\n'
            'def unrelated():\n    global value\n    value = os.environ\n'
            'def sample():\n    value = "safe"\n    m = {"x": "old"}\n'
            '    m["x"] = value\n    print(dict(m)["x"])\nsample()\n'))
        self.assertIsNone(self.inspect('import os\nvalue = "safe"\n'
            'def outer():\n    value = os.environ\n    def mutate():\n'
            '        nonlocal value\n        value = os.environ\n    mutate()\nouter()\n'
            'm = {"x": "old"}\nm["x"] = value\nprint(dict(m)["x"])\n'))
        self.assertIsNotNone(self.inspect('import os\ndef sample():\n    value = "safe"\n'
            '    def mutate():\n        nonlocal value\n        value = os.environ.get("HOME")\n'
            '    mutate()\n    m = {"x": "old"}\n    m["x"] = value\n'
            '    print(dict(m)["x"])\nsample()\n'))
        for binding, effect, loop in (
            ('key = "other"', 'global key\n    key = "x"', 'm[key] = os.environ'),
            ('domain = {"other": os.environ}', 'global domain\n    domain = {"x": os.environ}',
             'for key, value in domain.items():\n    m[key] = value'),
        ):
            with self.subTest(declared_domain=binding):
                self.assertIsNotNone(self.inspect('import os\n' + binding
                    + '\ndef mutate():\n    ' + effect + '\nmutate()\nm = {"x": "safe"}\n'
                    + loop + '\nprint(dict(m)["x"])\n'))

    def test_pr103_v22_frame_owned_literal_environment(self) -> None:
        for mutation, safe in (('', True), ('    alias = local\n', True),
                              ('    local["K"] = unknown\n', False),
                              ('    alias = local\n    alias["K"] = unknown\n', False),
                              ('    if flag:\n        local["K"] = unknown\n', False),
                              ('    alias = local\n    if flag:\n        alias = unknown\n', False),
                              ('    def retained(value=local):\n        pass\n', False),
                              ('    def retained():\n        return local\n', False),
                              ('    unknown(local)\n', False)):
            with self.subTest(local_environment=mutation):
                body = ('import subprocess\nm = {"x": "safe"}\n'
                    'def launch():\n    local = {"K": "V"}\n' + mutation
                    + '    process = subprocess.Popen(["printf", "ok"], env=local, stdout=subprocess.PIPE)\n'
                    '    stream = getattr(process, "stdout", None)\n    stream.close()\nlaunch()\n'
                    'subprocess.Popen(["printf", "ok"], env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_frame_owned_environment_forwarding(self) -> None:
        for effect, safe in (
            ('', True), ('    alias = environment\n', True),
            ('    environment["K"] = unknown\n', False),
            ('    alias = environment\n    alias["K"] = unknown\n', False),
            ('    environment = unknown\n', False),
            ('    unknown(environment)\n', False),
            ('    def retain(value=environment):\n        pass\n', False),
            ('    def retain():\n        return environment\n', False),
        ):
            with self.subTest(forwarded_environment=effect):
                body = ('import subprocess\nm = {"x": "safe"}\n'
                    'def forward(environment):\n' + effect
                    + '    process = subprocess.Popen(["printf", "ok"], env=environment, stdout=subprocess.PIPE)\n'
                    '    stream = getattr(process, "stdout", None)\n    stream.close()\n'
                    'def launch():\n    local = {"K": "V"}\n    forward(local)\n    forward(local)\n'
                    'launch()\nsubprocess.Popen(["printf", "ok"], env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_finite_native_argv_spreads(self) -> None:
        for provider, mutation, helper_mutation, safe in (
            ('(["ok"], ["reviewed"])', '', '', True),
            ('(["ok"], unknown)', '', '', False), ('unknown', '', '', False),
            ('(["ok"],)', '    arguments = unknown\n', '', False),
            ('(["ok"],)', '    alias = arguments\n    alias.append(unknown)\n', '', False),
            ('(["ok"],)', '', '    arguments.append(unknown)\n', False),
            ('(["ok"],)', '', '    alias = arguments\n    alias.append(unknown)\n', False),
            ('(["ok"],)', '', '    unknown(arguments)\n', False),
        ):
            with self.subTest(argv_iteration_provider=(provider, mutation, helper_mutation)):
                body = ('import subprocess\nm = {"x": "safe"}\n'
                    'def command(arguments):\n' + helper_mutation + '    return ["printf", *arguments]\n'
                    'for arguments in ' + provider + ':\n' + mutation
                    + '    subprocess.Popen(command(arguments), env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_constant_environment_native_run(self) -> None:
        for arguments, options, patch, safe in (
            ('["printf", "ok"]', ', text=True, capture_output=True, check=False', '', True),
            ('["printf", "ok"]', ', input=b"ok", timeout=1, stdout=subprocess.PIPE', '', True),
            ('unknown', '', '', False),
            ('["printf", "ok"]', ', preexec_fn=unknown', '', False),
            ('["printf", "ok"]', ', cwd=unknown', '', False),
            ('["printf", "ok"]', ', stdout=unknown', '', False),
            ('["printf", "ok"]', ', input=unknown', '', False),
            ('["printf", "ok"]', ', **unknown', '', False),
            ('["printf", "ok"]', '', 'subprocess.run = unknown\n', False),
            ('["printf", "ok"]', '', 'subprocess.Popen = unknown\n', False),
        ):
            with self.subTest(options=(arguments, options, patch)):
                body = ('import subprocess\n' + patch + 'm = {"x": "safe"}\n'
                    'def launch():\n    local = {"K": "V"}\n'
                    '    subprocess.run(' + arguments + ', env=local' + options + ')\n'
                    '    process = subprocess.Popen(["printf", "ok"], env=local, stdout=subprocess.PIPE)\n'
                    '    stream = getattr(process, "stdout", None)\n    stream.close()\n'
                    'launch()\nsubprocess.Popen(["printf", "ok"], env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_guarded_native_argv(self) -> None:
        for guard, safe in (
            ('    if not arguments or arguments[0] != "printf":\n        return\n', True),
            ('    if arguments[0] == "printf":\n        pass\n', True),
            ('    if arguments[unknown()] == "printf":\n        pass\n', False),
            ('    if arguments[0] == unknown():\n        pass\n', False),
            ('    unknown(arguments)\n', False),
            ('    alias = arguments\n    alias.append(unknown)\n', False),
            ('    arguments[0] = unknown\n', False),
        ):
            with self.subTest(guard=guard):
                body = ('import subprocess\nm = {"x": "safe"}\n'
                    'def launch(arguments, **kwargs):\n' + guard
                    + '    return subprocess.Popen(arguments, **kwargs)\n'
                    'launch(["printf", "ok"], env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_formatted_native_argument_types(self) -> None:
        for value, formatted, safe in (
            ('"reviewed"', 'f"HEAD:{value}"', True),
            ('"reviewed"', 'f"{value!r}"', True),
            ('3', 'f"{value:04}"', True),
            ('unknown', 'f"HEAD:{value}"', False),
            ('"reviewed"', 'f"{value:{unknown}}"', False),
        ):
            with self.subTest(formatted=(value, formatted)):
                body = ('import subprocess\nm = {"x": "safe"}\nvalue = ' + value
                    + '\nsubprocess.Popen(["printf", ' + formatted + '], env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_nonretaining_sequence_reads(self) -> None:
        for read, safe in (
            ('    size = len(arguments)\n', True),
            ('    index = 0\n    value = arguments[index]\n', True),
            ('    flag = "-C"\n    value = arguments[len(flag) + 1:]\n', True),
            ('    value = arguments[unknown()]\n', False),
            ('    value = arguments[unknown]\n', False),
            ('    value = unknown(arguments)\n', False),
            ('    value = arguments.pop()\n', False),
        ):
            with self.subTest(read=read):
                body = ('import subprocess\nm = {"x": "safe"}\n'
                    'def launch(arguments):\n' + read
                    + '    return subprocess.Popen(arguments, env=m)\n'
                    'launch(["printf", "ok"])\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_mapping_applicability_keeps_unknowns(self) -> None:
        for effect, safe in (
            ('', True), ('alias = values\n', True),
            ('unknown(values)\n', False), ('values["x"] = unknown\n', False),
            ('values["x"] = os.environ\n', False),
        ):
            with self.subTest(effect=effect):
                self.assertEqual(self.inspect('import os\nclass K:\n'
                    '    def __class_getitem__(cls, key):\n        return "safe"\n'
                    'values = {"x": K}\n' + effect
                    + 'ctor = dict\nprint(ctor(**values)["x"][0])\n') is None, safe)
        self.assertIsNotNone(self.inspect('values = {"x": unknown}\nprint(values["x"])\n'))
        for source in ('lambda: os.environ', 'lambda: str(os.environ)'):
            with self.subTest(callee=source):
                self.assertIsNotNone(self.inspect('import os\nfunctions = {"f": ' + source
                    + '}\nprint(functions["f"]())\n'))
        self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
            'def mutate():\n    m["x"] = os.environ\n    return "safe"\n'
            'functions = {"f": mutate}\nfunctions["f"]()\nprint(dict(m)["x"])\n'))
        for effect in ('functions["f"] = unknown\n', 'unknown(functions)\n',
                       'urllib.parse.quote = unknown\n', 'unknown()\n'):
            with self.subTest(encoder_effect=effect):
                self.assertIsNotNone(self.inspect('import urllib.parse\nfunctions = {"f": urllib.parse.quote}\n'
                    + effect + 'print(functions["f"]("safe"))\n'))
        self.assertIsNotNone(self.inspect('import os, builtins\n'
            'builtins.len = lambda value: os.environ\nfunctions = {"f": len}\n'
            'print(functions["f"]("safe"))\n'))

    def test_pr103_v23_conditional_key_requires_definite_overwrite(self) -> None:
        for initial, later, key, safe in (
            ('os.environ', '"safe"', '"x" if flag else "other"', False),
            ('"safe"', 'os.environ', '"x" if flag else "other"', False),
            ('"safe"', '"safe"', '"x" if flag else "other"', True),
            ('os.environ', '"safe"', '"x" if flag else "x"', True),
            ('os.environ', '"safe"', '"other"', False),
            ('os.environ', '"safe"', 'unknown', False),
        ):
            for selected in ('values', 'dict(values)', 'dict(**values)', 'dict(dict(values))'):
                with self.subTest(initial=initial, later=later, key=key, selected=selected):
                    body = ('import os\nflag = False\nvalues = {"x": ' + initial
                        + ', (' + key + '): ' + later + '}\nprint(' + selected + '["x"])\n')
                    self.assertEqual(self.inspect(body) is None, safe)

    def test_pr103_v23_function_namespace_handles_are_not_mapping_aliases(self) -> None:
        for setup, access in (
            ('', 'f.__globals__'),
            ('', 'getattr(f, "__globals__")'),
            ('', 'getattr(f, "__" + "globals__")'),
            ('lookup = getattr\n', 'lookup(f, "__globals__")'),
            ('member = "__globals__"\n', 'getattr(f, member)'),
            ('def member():\n    return "__globals__"\n', 'getattr(f, member())'),
            ('def lookup(fn, reader=getattr):\n    return reader(fn, "__globals__")\n', 'lookup(f)'),
            ('functions = {"f": f}\n', 'getattr(functions["f"], "__globals__")'),
            ('member = unknown\n', 'getattr(f, member)'),
            ('member = "safe" if flag else unknown\n', 'getattr(f, member)'),
            ('lookup = f.__getattribute__\n', 'lookup("__globals__")'),
            ('', 'object.__getattribute__(f, "__globals__")'),
            ('member = unknown\n', 'object.__getattribute__(f, member)'),
            ('member = unknown\n', 'getattr(f, "__getattribute__")(member)'),
        ):
            with self.subTest(setup=setup, access=access):
                self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
                    'def f():\n    pass\n' + setup + 'namespace = ' + access
                    + '\nnamespace["m"]["x"] = {"value": os.environ}\nprint(dict(m)["x"])\n'))
        self.assertIsNone(self.inspect('import ast\nnode = ast.parse("x")\n'
            'member = "body"\nvalue = getattr(node, member, None)\n'))
        self.assertIsNone(self.inspect('def f():\n    pass\nvalue = getattr(f, "__name__")\n'))

    def test_pr103_v24_wrapped_namespace_reflection_requires_closed_members(self) -> None:
        for setup, lookup in (
            ('', 'getattr(tuple([f])[0], "".join(["__", "globals__"]))'),
            ('member = "safe" if flag else unknown\n', 'getattr(tuple([f])[0], member)'),
            ('member = unknown\n', 'getattr(opaque(), member)'),
            ('lookup = tuple([getattr])[0]\n', 'lookup(f, "__globals__")'),
            ('lookup = getattr\nread = [lookup][0]\n', 'read(f, "__globals__")'),
            ('lookup = getattr(getattr, "__call__")\n', 'lookup(f, "__globals__")'),
            ('lookup = tuple([tuple([f])[0].__getattribute__])[0]\n',
                'lookup("".join(["__", "globals__"]))'),
        ):
            with self.subTest(setup=setup, lookup=lookup):
                self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
                    'def f():\n    pass\n' + setup + 'namespace = ' + lookup
                    + '\nnamespace["m"]["x"] = {"value": os.environ}\nprint(dict(m)["x"])\n'))
        for setup, member in (
            ('field = "body"\n', 'field'),
            ('field = "body" if flag else "orelse"\n', 'field'),
            ('field = "bo" + "dy"\n', 'field'),
        ):
            with self.subTest(safe_member=member, setup=setup):
                self.assertIsNone(self.inspect('import ast\nnode = ast.parse("x")\n'
                    + setup + 'value = getattr(node, ' + member + ', None)\n'))
        self.assertIsNone(self.inspect('import ast\nnode = ast.parse("x")\n'
            'for field in ("body", "orelse", "finalbody"):\n    value = getattr(node, field, None)\n'))
        for effect, body, safe in (
            ('', '    value = getattr(node, field, None)\n', True),
            ('fields[unknown] = None\n', '    value = getattr(node, field, None)\n', False),
            ('alias = fields\n', '    value = getattr(node, field, None)\n', False),
            ('opaque(fields)\n', '    value = getattr(node, field, None)\n', False),
            ('def mutate():\n    fields[unknown] = None\nmutate()\n',
                '    value = getattr(node, field, None)\n', False),
            ('', '    value = getattr(node, field, None)\n    fields[unknown] = None\n', False),
        ):
            with self.subTest(mapping_effect=effect, body=body):
                self.assertEqual(self.inspect('import ast\nnode = ast.parse("x")\n'
                    'fields = {"body": None, "orelse": None}\n' + effect
                    + 'for field in fields:\n' + body) is None, safe)

    def test_pr103_v25_member_returns_require_closed_unescaped_helpers(self) -> None:
        for effect, safe in (
            ('', True),
            ('reader_name = unknown\n', False),
            ('alias = reader_name\n', False),
            ('opaque(reader_name)\n', False),
            ('reader_name.__code__ = other.__code__\n', False),
            ('def capture(callback=reader_name):\n    pass\n', False),
        ):
            with self.subTest(effect=effect):
                self.assertEqual(self.inspect('import ast\nnode = ast.parse("x")\n'
                    'def reader_name():\n    return "body"\n' + effect
                    + 'value = getattr(node, reader_name(), None)\n') is None, safe)
        for value in ('"__globals__"', '"safe" if flag else unknown', 'unknown()'):
            with self.subTest(returned=value):
                self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
                    'def f():\n    pass\ndef reader_name():\n    return ' + value
                    + '\nnamespace = getattr(tuple([f])[0], reader_name())\n'
                    'namespace["m"]["x"] = {"value": os.environ}\nprint(dict(m)["x"])\n'))
        self.assertIsNone(self.inspect('import ast\nnode = ast.parse("x")\n'
            'def name():\n    return "body"\ndef forward():\n    return name()\n'
            'value = getattr(node, forward(), None)\n'))

    def test_pr103_v25_member_parameters_require_all_closed_callers(self) -> None:
        prefix = ('import ast\nnode = ast.parse("x")\n'
            'def read(receiver, member="body"):\n    return getattr(receiver, member, None)\n')
        for effect, safe in (
            ('read(node)\n', True),
            ('read(node, "body")\nread(node, member="orelse")\n', True),
            ('read(node, "body")\nread(node, unknown)\n', False),
            ('read(node, "body")\nread(*unknown)\n', False),
            ('read(node, "body")\nread(**unknown)\n', False),
            ('alias = read\nread(node)\n', False),
            ('opaque(read)\nread(node)\n', False),
            ('read.__code__ = other.__code__\nread(node)\n', False),
            ('def capture(callback=read):\n    pass\nread(node)\n', False),
        ):
            with self.subTest(effect=effect):
                self.assertEqual(self.inspect(prefix + effect) is None, safe)
        self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
            'def f():\n    pass\ndef read(receiver, member):\n    return getattr(receiver, member)\n'
            'namespace = read(tuple([f])[0], "".join(["__", "globals__"]))\n'
            'namespace["m"]["x"] = {"value": os.environ}\nprint(dict(m)["x"])\n'))

    def test_pr103_v27_class_owned_member_helpers_refuse_unknown_callers(self) -> None:
        for output in ('print(m)\n', 'print(dict(m)["x"])\n'):
            with self.subTest(output=output):
                self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
                    'def f():\n    pass\nclass C:\n'
                    '    def read(receiver, member="body"):\n        return getattr(receiver, member, None)\n'
                    '    read(None)\nnamespace = C.read(tuple([f])[0], "".join(["__", "globals__"]))\n'
                    'namespace["m"]["x"] = {"value": os.environ}\n' + output))

    def test_pr103_v27_definition_time_member_helper_ownership(self) -> None:
        for mutation in (
            'def mutate(name=name):\n    name.__code__ = evil.__code__\nmutate()\n',
            'def capture(ignored=(name := evil)):\n    pass\n',
            'class Holder:\n    name = name\nHolder.name.__code__ = evil.__code__\n',
        ):
            for output in ('print(m)\n', 'print(dict(m)["x"])\n'):
                with self.subTest(mutation=mutation, output=output):
                    self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
                        'def f():\n    pass\ndef name():\n    return "body"\n'
                        'def evil():\n    return "".join(["__", "globals__"])\n'
                        + mutation + 'namespace = getattr(tuple([f])[0], name())\n'
                        'namespace["m"]["x"] = {"value": os.environ}\n' + output))

    def test_pr103_v27_reflected_accessor_handles_cannot_lose_identity(self) -> None:
        for lookup in ('getattr(tuple([f])[0], "__getattribute__")',
                       'object.__getattribute__(tuple([f])[0], "__getattribute__")'):
            with self.subTest(lookup=lookup):
                self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
                    'def f():\n    pass\nlookup = ' + lookup
                    + '\nnamespace = lookup("".join(["__", "globals__"]))\n'
                    'namespace["m"]["x"] = {"value": os.environ}\nprint(m)\n'))

    def test_pr103_v28_comprehension_inputs_cannot_hide_helper_escape(self) -> None:
        for box, selected in (
            ('[name for name in [name]]', 'box[0]'),
            ('{0: name for name in [name]}', 'box[0]'),
            ('{name for name in [name]}', 'next(iter(box))'),
            ('(name for name in [name])', 'next(box)'),
        ):
            with self.subTest(box=box):
                self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
                    'def f():\n    pass\ndef name():\n    return "body"\n'
                    'def evil():\n    return "".join(["__", "globals__"])\n'
                    'box = ' + box + '\n' + selected + '.__code__ = evil.__code__\n'
                    'namespace = getattr(tuple([f])[0], name())\n'
                    'namespace["m"]["x"] = {"value": os.environ}\nprint(m)\n'))

    def test_pr103_v28_declarations_preserve_helper_binding_owners(self) -> None:
        for definition, mutation, selection in (
            ('def name():\n    return "body"\n',
                'global name\n    name.__code__ = evil.__code__',
                'getattr(tuple([f])[0], name())'),
            ('def read(receiver, member="body"):\n    return getattr(receiver, member, None)\n',
                'global read\n    read.__defaults__ = ("__globals__",)', 'read(tuple([f])[0])'),
        ):
            with self.subTest(mutation=mutation):
                self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
                    'def f():\n    pass\ndef evil():\n    return "".join(["__", "globals__"])\n'
                    + definition + 'def mutate():\n    ' + mutation + '\nmutate()\n'
                    'namespace = ' + selection + '\nnamespace["m"]["x"] = {"value": os.environ}\nprint(m)\n'))
        self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
            'def f():\n    pass\ndef outer():\n'
            '    def name():\n        return "body"\n'
            '    def evil():\n        return "".join(["__", "globals__"])\n'
            '    def mutate():\n        nonlocal name\n        name.__code__ = evil.__code__\n'
            '    mutate()\n    namespace = getattr(tuple([f])[0], name())\n'
            '    namespace["m"]["x"] = {"value": os.environ}\n    print(m)\nouter()\n'))
        self.assertIsNone(self.inspect('import ast\ndef name():\n    return "body"\n'
            'def observe():\n    global name\n    return name()\nobserve()\n'
            'value = getattr(ast.parse("x"), name(), None)\n'))

    def test_pr103_v28_namespace_callables_cannot_escape_as_data(self) -> None:
        for setup, lookup in (
            ('readers = {"g": globals}\n', 'readers["g"]()'),
            ('readers = {"g": locals}\n', 'readers["g"]()'),
            ('readers = {"g": vars}\n', 'readers["g"]()'),
            ('empty = ()\n', 'vars(*empty)'),
            ('', 'getattr(len.__self__, "globals")()'),
            ('', 'len.__self__.globals()'),
            ('', 'getattr(len.__self__, "vars")()'),
            ('', 'len.__self__.vars()'),
        ):
            with self.subTest(setup=setup, lookup=lookup):
                self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
                    + setup + 'namespace = ' + lookup + '\n'
                    'namespace["m"]["x"] = {"value": os.environ}\nprint(m)\n'))
        self.assertIsNone(self.inspect('import ast\nvalue = vars(ast.parse("x"))\n'))

    def test_pr103_v30_control_flow_methods_do_not_capture_class_bindings(self) -> None:
        prefix = ('import os\nm = {"x": "safe"}\ndef f():\n    pass\n'
            'def name():\n    return "body"\n'
            'def evil():\n    return "".join(["__", "globals__"])\n')
        for guard in ('if True:', 'try:', 'for unused in (None,):'):
            with self.subTest(guard=guard):
                method = ('class C:\n    name = "safe"\n    ' + guard + '\n'
                    '        def mutate():\n            name.__code__ = evil.__code__\n')
                if guard == 'try:':
                    method += '    finally:\n        pass\n'
                self.assertIsNotNone(self.inspect(prefix + method + 'C.mutate()\n'
                    'namespace = getattr(tuple([f])[0], name())\n'
                    'namespace["m"]["x"] = {"value": os.environ}\nprint(m)\n'))
        self.assertIsNone(self.inspect('import ast\ndef name():\n    return "body"\n'
            'class C:\n    name = "safe"\n    if True:\n        def read():\n            return name()\n'
            'value = getattr(ast.parse("x"), name(), None)\nC.read()\n'))

    def test_pr103_v30_unindexed_class_header_rebinding_refuses_outer_proof(self) -> None:
        self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
            'def f():\n    pass\ndef name():\n    return "body"\n'
            'def evil():\n    return "".join(["__", "globals__"])\n'
            'def outer():\n    class C:\n        global name\n'
            '        def capture(unused=(name := evil)):\n            pass\nouter()\n'
            'namespace = getattr(tuple([f])[0], name())\n'
            'namespace["m"]["x"] = {"value": os.environ}\nprint(m)\n'))

    def test_pr103_v26_scanner_runtime_type_admission(self) -> None:
        tree = ast.parse(_scanner_module_source(PACKET_TEXT))
        self.assertIsNone(self.scanner['python_subprocess_os_reexport_violation'](tree))

    def test_pr103_v29_primitive_call_launcher_admission(self) -> None:
        source_tree = ast.parse(_scanner_module_source(PACKET_TEXT))
        helper = next(node for node in ast.walk(source_tree)
                      if isinstance(node, ast.FunctionDef) and node.name == 'v22_primitive_call')
        tree = ast.Module(body=[helper], type_ignores=[])
        modules, functions, _unresolved = self.scanner['python_import_bindings'](tree)
        self.assertIsNone(self.scanner['python_class_command_attribute_violation'](tree, modules, functions))

    def test_pr103_v22_dynamic_lookup_complete_key_domain(self) -> None:
        for effect, safe in (
            ('', True), ('defaults["OTHER"] = "safe"\n', True),
            ('defaults["OTHER"] = os.environ\n', False),
            ('defaults.update(os.environ)\n', False),
            ('unknown(defaults)\n', False),
        ):
            with self.subTest(effect=effect):
                self.assertEqual(self.inspect('import os\ndefaults = {"PATH": "/x"}\n'
                    + effect + 'for key in os.environ:\n'
                    '    print(defaults[key] if key in defaults else "none")\n') is None, safe)
        self.assertIsNotNone(self.inspect('import os\ndefaults = {"PATH": "/x"}\n'
            'for key in os.environ:\n    key = unknown\n'
            '    print(defaults[key] if key in defaults else "none")\n'))
        self.assertIsNotNone(self.inspect('defaults = {"PATH": "/x"}\nprint(defaults[unknown])\n'))

    def test_pr103_v22_actual_frames_ignore_unrelated_parameters(self) -> None:
        for use, safe in (
            ('def unrelated(command):\n    return command\nunrelated("safe")\n', True),
            ('def unrelated(command=command):\n    return command\n', False),
            ('def unrelated(*, command=command):\n    return command\n', False),
            ('alias = command\n', False),
            ('holder = [command]\n', False),
        ):
            with self.subTest(use=use):
                body = ('import subprocess\nm = {"x": "safe"}\n'
                    'def command(arguments):\n    return ["printf", *arguments]\n' + use
                    + 'subprocess.Popen(command(["ok"]), env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_native_filesystem_string_types(self) -> None:
        for argument, safe in (('b"reviewed"', True), ('"reviewed"', True), ('unknown', False), ('os.environ', False)):
            with self.subTest(argument=argument):
                body = ('import os\nimport subprocess\nm = {"x": "safe"}\n'
                    'subprocess.Popen(["printf", os.fsdecode(' + argument + ')], env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_native_filtered_sequence_types(self) -> None:
        for source, value, effect, safe in (
            ('b"one\\ttwo".split(b"\\t")', 'part', '', True),
            ('unknown', 'part', '', False),
            ('["safe"]', 'unknown', '', False),
            ('["safe"]', 'part', 'parts[0] = unknown\n', False),
        ):
            with self.subTest(sequence=(source, value, effect)):
                body = ('import os\nimport subprocess\nm = {"x": "safe"}\n'
                    'parts = [' + value + ' for part in ' + source + ' if part]\n' + effect
                    + 'for record in parts:\n    subprocess.Popen(["printf", os.fsdecode(record)], env=m)\n'
                    'print(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_native_run_result_requires_effects(self) -> None:
        for arguments, environment, options, safe in (
            ('["printf", "ok"]', '{"K": "V"}', ', capture_output=True, text=True', True),
            ('unknown', '{"K": "V"}', ', capture_output=True', False),
            ('["printf", "ok"]', 'unknown', ', capture_output=True', False),
            ('["printf", "ok"]', '{"K": "V"}', ', preexec_fn=unknown, capture_output=True', False),
            ('["printf", "ok"]', '{"K": "V"}', ', input=unknown, capture_output=True', False),
        ):
            with self.subTest(result=(arguments, environment, options)):
                body = ('import subprocess\nm = {"x": "safe"}\n'
                    'result = subprocess.run(' + arguments + ', env=' + environment + options + ')\n'
                    'parts = [part for part in result.stdout.split(":") if part]\n'
                    'for part in parts:\n    subprocess.Popen(["printf", part], env=m)\n'
                    'print(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_buffer_completed_result_boundary(self) -> None:
        for argv, status, safe in (('["printf", "ok"]', '0', True), ('[m]', '0', False), ('["printf", "ok"]', 'unknown', False)):
            with self.subTest(completed=(argv, status)):
                body = ('import os\nimport subprocess\nm = {"x": "safe"}\n'
                    'def capture(process):\n    stream = getattr(process, "stdout", None)\n'
                    '    data = bytearray()\n    if stream is not None:\n        data.extend(os.read(stream.fileno(), 64))\n'
                    '    stdout = bytes(data)\n    return subprocess.CompletedProcess(' + argv + ', ' + status + ', stdout, b"")\n'
                    'p = subprocess.Popen(["printf", "ok"], env=m, stdout=subprocess.PIPE)\nresult = capture(p)\n'
                    'subprocess.Popen(["printf", result.stdout], env=m)\n'
                    'print(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_completed_result_buffer_fields(self) -> None:
        for value, safe in (('b"safe"', True), ('"text"', False), ('unknown', False), ('os.environ', False)):
            with self.subTest(value=value):
                body = ('import os\nimport subprocess\nm = {"x": "safe"}\n'
                    'result = subprocess.CompletedProcess([], 0, stdout=' + value + ')\n'
                    'data = bytearray()\ndata.extend(result.stdout)\n'
                    'subprocess.Popen(["printf", os.fsdecode(bytes(data))], env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_borrowed_native_buffers(self) -> None:
        for body, other, safe in (
            ('    data.clear()\n', '', True),
            ('    retained.append(data)\n', '', False),
            ('    return data\n', '', False),
            ('    def keep(value=data):\n        pass\n', '', False),
            ('    data.clear()\n', 'consume(unknown)\n', False),
        ):
            with self.subTest(borrow=(body, other)):
                source = ('import os\nimport subprocess\nm = {"x": "safe"}\nretained = []\n'
                    'def consume(data):\n' + body
                    + 'def capture(process):\n    stream = getattr(process, "stdout", None)\n'
                    '    data = bytearray()\n    if stream is not None:\n        data.extend(os.read(stream.fileno(), 64))\n'
                    '    consume(data)\n    return bytes(data)\n'
                    'p = subprocess.Popen(["printf", "ok"], env=m, stdout=subprocess.PIPE)\n'
                    'result = subprocess.CompletedProcess([], 0, stdout=capture(p))\n' + other
                    + 'subprocess.Popen(["printf", result.stdout], env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(source)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_native_buffer_decode_boundary(self) -> None:
        for encoding, safe in (('"utf-8"', True), ('unknown', False)):
            with self.subTest(encoding=encoding):
                body = ('import os\nimport subprocess\nm = {"x": "safe"}\n'
                    'def capture(process):\n    stream = getattr(process, "stdout", None)\n'
                    '    data = bytearray()\n    if stream is not None:\n        data.extend(os.read(stream.fileno(), 64))\n'
                    '    stdout = bytes(data)\n    stdout = stdout.decode(' + encoding + ')\n'
                    '    return subprocess.CompletedProcess([], 0, stdout, "")\n'
                    'p = subprocess.Popen(["printf", "ok"], env=m, stdout=subprocess.PIPE)\nresult = capture(p)\n'
                    'subprocess.Popen(["printf", result.stdout], env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_nonstandard_class_domain_refuses(self) -> None:
        for definition, value in (
            ('class K:\n    def __new__(cls):\n        return os.environ\n', 'K()'),
            ('def replace(cls):\n    return os.environ\n@replace\nclass K:\n    pass\n', 'K'),
            ('def replace(*args, **kwargs):\n    return os.environ\nclass K(metaclass=replace):\n    pass\n', 'K'),
            ('class K:\n    pass\nK.__new__ = staticmethod(lambda cls: os.environ)\n', 'K()'),
            ('class K:\n    pass\nsetattr(K, "__new__", staticmethod(lambda cls: os.environ))\n', 'K()'),
            ('def factory(cls):\n    return {"HOME": os.environ.get("HOME")}\nclass K:\n    __new__ = factory\n', 'K()'),
            ('def factory(cls):\n    return {"HOME": os.environ.get("HOME")}\nclass K:\n    for __new__ in [factory]:\n        pass\n', 'K()'),
            ('import builtins\nclass K:\n    pass\nbuiltins.setattr(K, "__new__", staticmethod(lambda cls: os.environ))\n', 'K()'),
            ('from builtins import setattr as install\nclass K:\n    pass\ninstall(K, "__new__", staticmethod(lambda cls: os.environ))\n', 'K()'),
        ):
            with self.subTest(definition=definition):
                self.assertIsNotNone(self.inspect('import os\n' + definition
                    + 'values = {"x": ' + value + '}\nprint(values["x"]["HOME"])\n'))

    def test_pr103_v22_update_provider_state_snapshot(self) -> None:
        for mutation, safe in (('', True), ('alias = patches\n', True),
                              ('seen = patches.get("other")\n', True),
                              ('checked = "other" in patches\n', True),
                              ('patches["x"] = os.environ\n', False),
                              ('alias = patches\nalias["x"] = os.environ\n', False),
                              ('unknown(patches)\n', False)):
            with self.subTest(update_provider=mutation):
                self.assertEqual(self.inspect('import os\nm = {"x": "safe"}\n'
                    'patches = {"other": os.environ}\n' + mutation
                    + 'm.update(patches)\nprint(dict(m)["x"])\n') is None, safe)
        self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\nkey = "x"\n'
            'patches = {key: os.environ}\nkey = "other"\nm.update(patches)\nprint(dict(m)["x"])\n'))
        self.assertIsNone(self.inspect('import os\nm = {"x": os.environ}\nkey = "x"\n'
            'patches = {key: "safe"}\nkey = "other"\nm.update(patches)\nprint(dict(m)["x"])\n'))

    def test_pr103_v22_unsupported_subscript_mutations(self) -> None:
        for mutation in ('m["x"] += os.environ.get("HOME", "")',
                         'm["x"] += "harmless"', 'del m["x"]',
                         'm["x"], other = (os.environ.get("HOME"), None)'):
            with self.subTest(unsupported_subscript_mutation=mutation):
                self.assertIsNotNone(self.inspect('import os\nm = {"x": "safe"}\n'
                    + mutation + '\nprint(dict(m)["x"])\n'))
        self.assertIsNone(self.inspect('m = {"x": "old"}\nm["x"] = "safe"\nprint(dict(m)["x"])\n'))

    def test_pr103_v22_native_capture_buffer_result_types(self) -> None:
        for mutation, safe in (('data.extend(os.read(stream.fileno(), 64))', True),
                               ('data.extend(os.read(stream.fileno(), chunk_size))', True),
                               ('chunk = os.read(stream.fileno(), 64)\n        data.extend(chunk)', True),
                               ('data.extend(os.read(stream.fileno(), 64 * unknown))', False),
                               ('data.extend(unknown)', False), ('data.append(unknown)', False)):
            with self.subTest(buffer_mutation=mutation):
                body = ('import os\nimport subprocess\nchunk_size = 64 * 1024\nm = {"x": "safe"}\n'
                    'def capture(process):\n    stream = getattr(process, "stdout", None)\n'
                    '    data = bytearray()\n    if stream is not None:\n        ' + mutation
                    + '\n    return bytes(data)\n'
                    'p = subprocess.Popen(["printf", "ok"], env=m, stdout=subprocess.PIPE)\n'
                    'result = subprocess.CompletedProcess([], 0, stdout=capture(p))\n'
                    'subprocess.Popen(["printf", result.stdout], env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_nearer_loop_value_binders(self) -> None:
        for source, safe in (('[os.environ.get("HOME")]', False), ('["reviewed"]', True),
                             ('unknown', False)):
            with self.subTest(loop_value=source):
                self.assertEqual(self.inspect('import os\nvalue = "safe"\nm = {"x": "safe"}\n'
                    'for value in ' + source + ':\n    m["x"] = value\nprint(dict(m)["x"])\n') is None, safe)
        for binding in (
            'with unknown as value:\n    m["x"] = value',
            'match os.environ.get("HOME"):\n    case value:\n        m["x"] = value',
            '[m.__setitem__("x", value) for value in [os.environ.get("HOME")]]',
            'for value in ["reviewed"]:\n    value = os.environ.get("HOME")\n    m["x"] = value',
            'value, other = (os.environ.get("HOME"), None)\nm["x"] = value',
            'del value\nm["x"] = value',
        ):
            with self.subTest(nearer_binder=binding):
                self.assertIsNotNone(self.inspect('import os\nvalue = "safe"\nm = {"x": "safe"}\n'
                    + binding + '\nprint(dict(m)["x"])\n'))

    def test_pr103_v22_comprehension_iteration_source_points(self) -> None:
        for source, safe in (('{"x"}', True), ('unknown', False)):
            with self.subTest(iteration_source=source):
                self.assertEqual(self.inspect('m = {"x": "safe"}\nfixture = ' + source
                    + '\nchecked = any(key in m for key in fixture)\nprint(dict(m)["x"])\n') is None, safe)
        for mutation in ('alias = fixture\nalias.add(unknown)', 'unknown(fixture)',
                         'fixture.add(unknown)'):
            with self.subTest(iteration_provider_mutation=mutation):
                self.assertIsNotNone(self.inspect('m = {"x": "safe"}\nfixture = {"x"}\n'
                    + mutation + '\nchecked = any(key in m for key in fixture)\nprint(dict(m)["x"])\n'))

    def test_pr103_v22_codec_and_import_hook_refusals(self) -> None:
        for hook in (
            'import codecs\ncodecs.register(unknown)',
            'import codecs\ncodecs.register_error("custom", unknown)',
            'import sys\nsys.path.insert(0, "custom")',
            'import sys\nsystem = sys\nsearch = system.path\nsearch.append("custom")',
            'import sys\nsys.meta_path.append(unknown)',
            'import sys\nsys.path_hooks.append(unknown)',
            'import sys\nsys.path_importer_cache.clear()',
            'import encodings\nencodings.search_function = unknown',
        ):
            with self.subTest(codec_hook=hook):
                body = ('import subprocess\n' + hook + '\nm = {"x": "safe"}\n'
                    'command = ["printf", b"ok".decode("utf-8")]\n'
                    'subprocess.Popen(command, env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertIsNotNone(self.scanner['python_sensitive_read_violation'](tree, parents))

    def test_pr103_v22_native_parameter_all_callers(self) -> None:
        for signature, invocation, safe in (
            ('value=unknown', 'selected()', False),
            ('*, value=unknown', 'selected()', False),
            ('value="literal"', 'selected()', True),
        ):
            with self.subTest(default_parameter=signature):
                body = ('import subprocess\nm = {"x": "safe"}\n'
                    'def selected(' + signature + '):\n    return value\n'
                    'command = ["printf", selected(value="provided"), ' + invocation + ']\n'
                    'subprocess.Popen(command, env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_fresh_keyword_pop_native_types(self) -> None:
        for arguments, mutation, safe in (
            ('', '', True), (', encoding="ascii"', '', True),
            ('', '    kwargs["stdout"] = subprocess.PIPE\n', True),
            (', encoding=unknown', '', False), ('', '    kwargs["encoding"] = unknown\n', False),
            ('', '    alias = kwargs\n    unknown(alias)\n', False),
        ):
            with self.subTest(keyword_pop=(arguments, mutation)):
                body = ('import subprocess\nm = {"x": "safe"}\n'
                    'def decoded(value, **kwargs):\n' + mutation
                    + '    encoding = kwargs.pop("encoding", None) or "utf-8"\n'
                    '    return value.decode(encoding)\n'
                    'command = ["printf", decoded(b"ok"' + arguments + ')]\n'
                    'subprocess.Popen(command, env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_native_capture_mapping_projection(self) -> None:
        for mutation, safe in (('', True), ('    captures["stdout"] = unknown\n', False),
                               ('    alias = captures\n    alias["stdout"] = unknown\n', False),
                               ('    unknown(captures)\n', False)):
            with self.subTest(capture_mapping=mutation):
                body = ('import os\nimport subprocess\nm = {"x": "safe"}\n'
                    'def capture(process):\n    stream = getattr(process, "stdout", None)\n'
                    '    captures = {"stdout": bytearray(), "stderr": bytearray()}\n' + mutation
                    + '    data = captures["stdout"]\n    if stream is not None:\n'
                    '        data.extend(os.read(stream.fileno(), 64))\n'
                    '    return bytes(captures["stdout"])\n'
                    'p = subprocess.Popen(["printf", "ok"], env=m, stdout=subprocess.PIPE)\n'
                    'result = subprocess.CompletedProcess([], 0, stdout=capture(p))\n'
                    'subprocess.Popen(["printf", result.stdout], env=m)\nprint(dict(m)["x"])\n')
                tree = ast.parse(body)
                parents = {child: parent for parent in ast.walk(tree) for child in ast.iter_child_nodes(parent)}
                self.assertEqual(self.scanner['python_sensitive_read_violation'](tree, parents) is None, safe)

    def test_pr103_v22_standard_pipe_selector_dependencies(self) -> None:
        prefix = ('import os\nimport subprocess\nimport selectors\nm = {"x": "safe"}\n'
            'p = subprocess.Popen(["printf", "ok"], env=m, stdout=subprocess.PIPE)\n'
            'stream = getattr(p, "stdout", None)\n')
        for factory, data, safe in (('selectors.DefaultSelector()', '"stdout"', True),
                                     ('unknown', '"stdout"', False),
                                     ('selectors.DefaultSelector()', 'unknown', False)):
            with self.subTest(factory=factory, data=data):
                self.assertEqual(self.inspect(prefix + 'selector = ' + factory + '\n'
                    'if stream is not None:\n    os.set_blocking(stream.fileno(), False)\n'
                    '    selector.register(stream, selectors.EVENT_READ, ' + data + ')\n'
                    '    selector.unregister(stream)\n    stream.close()\nselector.close()\n'
                    'print(dict(m)["x"])\n') is None, safe)
        for ready, safe in (('selector.select(1)', True), ('unknown', False)):
            with self.subTest(selector_keys=ready):
                self.assertEqual(self.inspect(prefix
                    + 'selector = selectors.DefaultSelector()\n'
                    'selector.register(stream, selectors.EVENT_READ, "stdout")\n'
                    'ready = ' + ready + '\nfor key, _ in ready:\n'
                    '    stream = key.fileobj\n    chunk = os.read(stream.fileno(), 64)\n'
                    '    selector.unregister(stream)\n    stream.close()\nselector.close()\n'
                    'print(dict(m)["x"])\n') is None, safe)
        for timeout, safe in (('1', True), ('max(0, 1 - 0.5)', True),
                              ('min(0.5, 1)', True), ('max(0, unknown)', False), ('unknown', False)):
            with self.subTest(process_timeout=timeout):
                self.assertEqual(self.inspect(prefix + 'p.wait(timeout=' + timeout
                    + ')\nprint(dict(m)["x"])\n') is None, safe)

    def test_pr103_v22_callable_namespace_capabilities(self) -> None:
        for setup in (
            'argv = ["printf", "ok"]\nalias = argv\nalias.append(unknown)',
            '@unknown\ndef helper():\n    pass\nargv = ["printf", "ok"]',
            'unknown.touch()\nargv = ["printf", "ok"]',
            'os.environ.update(unknown)\nargv = ["printf", "ok"]',
        ):
            with self.subTest(setup=setup):
                self.assertIsNotNone(self.inspect('import os\nimport subprocess\n' + setup
                    + '\nm = {"x": "safe"}\nsubprocess.Popen(argv, env=m)\n'
                    'print(dict(m)["x"])\n'))
        for module, safe in (('unicodedata', True), ('unknown_extension', False)):
            with self.subTest(module=module):
                self.assertEqual(self.inspect('import subprocess\nimport ' + module
                    + '\nvalue = ' + module + '.normalize("NFC", "safe")\n'
                    'm = {"x": "safe"}\nsubprocess.Popen(["printf", "ok"], env=m)\n'
                    'print(dict(m)["x"])\n') is None, safe)
        for replacement in ('os.environ.get("HOME")', '"harmless"'):
            for mutation in (
                'subprocess.Popen.__init__.__globals__.update({"Popen": fake})',
                'constructor = subprocess.Popen\ninitializer = constructor.__init__\n'
                'namespace = initializer.__globals__\nnamespace.update({"Popen": fake})',
                'initializer = subprocess.Popen.__init__\n'
                'namespace = getattr(initializer, "__globals__")\n'
                'namespace["Popen"] = fake',
            ):
                with self.subTest(replacement=replacement, mutation=mutation):
                    self.assertIsNotNone(self.inspect('import os\nimport subprocess\n'
                        'def fake(*args, **kwargs):\n    kwargs["env"]["x"] = '
                        + replacement + '\n' + mutation
                        + '\nm = {"x": "old"}\nm["x"] = "safe"\n'
                        'subprocess.Popen(["printf", "ok"], env=m)\n'
                        'print(dict(m)["x"])\n'))
        for member in ('__defaults__', '__kwdefaults__', '__closure__', '__code__', '__dict__'):
            with self.subTest(member=member):
                self.assertIsNotNone(self.inspect('import subprocess\n'
                    'state = subprocess.Popen.__init__.' + member + '\n'
                    'm = {"x": "safe"}\nsubprocess.Popen(["printf", "ok"], env=m)\n'
                    'print(dict(m)["x"])\n'))

    def test_pr103_v22_mutable_values_and_unknown_constructors(self) -> None:
        for body in (
            'import os\nvalue = []\nm = {"x": "safe"}\nm["x"] = value\n'
            'c = dict(m)\nvalue.append(os.environ)\nprint(c["x"])\n',
            'import os\nm = {"x": "safe"}\nm["x"] = os.environ\n'
            'constructor = dict if flag else unknown\nprint(constructor(m)["x"])\n',
            'import os\nm = {"x": "safe"}\nm["x"] = os.environ\n'
            'def read():\n    return dict(m)["x"]\nm = unknown\nprint(read())\n',
        ):
            with self.subTest(body=body):
                self.assertIsNotNone(self.inspect(body))

    def test_pr103_v13_factory_returned_protocol_classes(self) -> None:
        consumer = ('async def collect():\n    async for item in K():\n'
                    '        print(item)\n        break\ncollect().send(None)\n')
        for value in ('os.environ', '"safe"'):
            with self.subTest(value=value, boundary='nonlocal factory'):
                self.assertIsNotNone(self.inspect('import os\nclass L:\n'
                    '    async def __anext__(self):\n        return ' + value + '\n'
                    'def build():\n    saved = None\n    def mutate():\n'
                    '        object.__setattr__(saved, "__class__", L)\n'
                    '    class K:\n        def __aiter__(self):\n            nonlocal saved\n'
                    '            saved = self\n            mutate()\n            return self\n'
                    '        async def __anext__(self):\n            return "safe"\n'
                    '    return K\nK = build()\n' + consumer))
        for returned in ('Local', 'alias', 'Local if flag else Other',
                         'Local if flag else unknown', 'build()'):
            with self.subTest(returned=returned):
                self.assertEqual(self.inspect('flag = True\ndef build():\n'
                    '    class Local:\n        def __aiter__(self):\n            return self\n'
                    '        async def __anext__(self):\n            return "safe"\n'
                    '    class Other:\n        def __aiter__(self):\n            return self\n'
                    '        async def __anext__(self):\n            return "safe"\n'
                    '    alias = Local\n    return ' + returned + '\nK = build()\n'
                    + consumer) is None, returned not in ('Local if flag else unknown', 'build()'))

    def test_pr103_v13_assigned_await_all_targets(self) -> None:
        consumer = ('class L:\n    def __anext__(self):\n        return Awaitable()\n'
                    'class K:\n    def __aiter__(self):\n        return L()\n'
                    'async def collect():\n    async for item in K():\n'
                    '        print(item)\n        break\ncollect().send(None)\n')
        for value in ('os.environ', '"safe"'):
            for target in ('leak if flag else safe', 'alias', 'leak if flag else unknown'):
                with self.subTest(value=value, target=target):
                    self.assertEqual(self.inspect('import os\n'
                        'def leak(self):\n    if False:\n        yield None\n    return ' + value + '\n'
                        'def safe(self):\n    if False:\n        yield None\n    return "safe"\n'
                        'flag = True\nalias = leak if flag else safe\nclass Awaitable: pass\n'
                        'Awaitable.__await__ = ' + target + '\n' + consumer) is None,
                        value == '"safe"' and 'unknown' not in target)

    def test_pr103_v13_class_namespace_callback_escape(self) -> None:
        for value in ('str(os.environ)', '"safe"'):
            for assignment in ('__call__ = install', 'first = install\n    __call__ = first'):
                with self.subTest(value=value, assignment=assignment):
                    result = self.inspect('import os\nclass K: pass\n'
                        'def leak(self):\n    return ' + value + '\n'
                        'def install(receiver, name="label"):\n    setattr(K, name, leak)\n'
                        'class Callback:\n    ' + assignment + '\ninstall(None)\n'
                        'list(map(Callback(), ["__str__"]))\nprint(K())\n')
                    self.assertIsNotNone(result)
                    self.assertIn('unresolved member name', result)
            for calls in ('install(None)', 'first = install\nalias = first\nalias(None)',
                          'first = install\nalias = first\nalias(None, "__str__")'):
                self.assertEqual(self.inspect('import os\nclass K: pass\n'
                    'def leak(self):\n    return ' + value + '\n'
                    'def install(receiver, name="label"):\n    setattr(K, name, leak)\n'
                    + calls + '\nprint(K())\n') is None,
                    value == '"safe"' or '"__str__"' not in calls)

    def test_pr103_four_classmethod_decorator_aliases(self) -> None:
        for setup, decorator in (('cm = classmethod', 'cm'),
                                 ('first = classmethod\ncm = first', 'cm')):
            for invocation in ('E.invoke(C)', 'E().invoke(C)', 'fn = E.invoke\nfn(C)', 'E.invoke(target=C)'):
                for source in ('os.system', 'len'):
                    with self.subTest(setup=setup, invocation=invocation, source=source):
                        result = self.inspect('import os\n' + setup + '\nclass C:\n    step = ' + source
                            + '\nclass E:\n    @' + decorator
                            + '\n    def invoke(cls, target):\n        target.step("safe")\n'
                            + invocation + '\n')
                        self.assertIsNotNone(result)
                        self.assertIn("unreviewed decorator", result)

    def test_pr103_projection_bookkeeping_preserves_nested_helper_calls(self) -> None:
        scanner_tree = ast.parse(_scanner_module_source(PACKET_TEXT))
        projection = next(node for node in scanner_tree.body if isinstance(node, ast.FunctionDef)
                          and node.name == 'python_destructured_binding_values')
        original_helper = ('def check():\n    def source_value(value):\n        return False\n'
                           '    return source_value(None)\nresult = check()\n')
        self.assertIsNone(self.inspect('import ast\n' + ast.unparse(projection) + '\n' + original_helper))
        # The conservative unresolved-destructuring rule remains unchanged;
        # only the actual scanner's new bookkeeping avoids this collision.
        self.assertIsNotNone(self.inspect(original_helper + 'def project(items):\n    pattern, source_value = items.pop()\n'))

    def test_current_packet_has_no_static_scanner_violations(self) -> None:
        matches: list[str] = []
        shell_command_count = 0
        for command, number in self.scanner["shell_commands"](PACKET_TEXT):  # type: ignore[operator]
            shell_command_count += 1
            if self.scanner["shell_process_substitution"](command):  # type: ignore[operator]
                matches.append(f"line {number}: shell process substitutions are not allowed")
                continue
            for segment in self.scanner["shell_token_segments"](command):  # type: ignore[operator]
                if self.scanner["python_stdin_command"](segment):  # type: ignore[operator]
                    if self.scanner["reviewed_python_heredoc_segment"](command, segment):  # type: ignore[operator]
                        continue
                    matches.append(
                        f"line {number}: Python stdin/heredoc execution must be "
                        "an isolated AST-inspected heredoc"
                    )
                    continue
                violation = self.scanner["forbidden_shell_command"](segment)  # type: ignore[operator]
                if violation:
                    matches.append(f"line {number}: {violation}")

        python_body_count = 0
        for number, body, safe_marker, invocation in self.scanner["python_heredoc_bodies"](PACKET_TEXT):  # type: ignore[operator]
            python_body_count += 1
            if not invocation["isolated"]:
                matches.append(
                    f"line {number}: executable Python heredoc must use -I before body inspection"
                )
                continue
            if not invocation["quoted"]:
                matches.append(
                    f"line {number}: executable Python heredoc must quote its delimiter before body inspection"
                )
                continue
            if invocation["interpreter"] != "/opt/homebrew/bin/python3":
                matches.append(
                    f"line {number}: executable Python heredoc must use absolute /opt/homebrew/bin/python3"
                )
                continue
            violation = self.scanner["inspect_python_heredoc"](body, safe_marker)  # type: ignore[operator]
            if violation:
                matches.append(f"line {number}: {violation}")

        self.assertGreater(shell_command_count, 0)
        self.assertGreater(python_body_count, 0)
        self.assertEqual([], matches, "\n".join(matches))
        print(
            f"current packet static scan: {shell_command_count} shell commands, "
            f"{python_body_count} Python heredoc bodies, zero violations"
        )


if __name__ == "__main__":
    try:
        _require_scanner_ast_runtime()
    except RuntimeError as error:
        sys.exit(str(error))
    unittest.main(verbosity=2)
