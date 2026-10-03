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
            '        return print\n'
            'launch = launcher_factory()\n'
            'launch(["gh", "workflow", "run", "ci.yml"])\n'
        )
        self.assertIsNotNone(self.inspect(body))

        safe = (
            'def value_factory():\n'
            '    return print\n'
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

        origin_capture = _top_level_assignment(self.verification, "origin_result")
        self.assertEqual(
            self.scanner["python_dotted_name"](origin_capture.value.func),  # type: ignore[operator,union-attr]
            "run_bounded_git_query",
        )
        self.assertIn("remote.origin.url", ast.unparse(origin_capture.value))
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
            globals()["PACKET_TEXT"] = original.replace(
                "def inspect_python_heredoc(body, safe_marker):",
                'probe = os.system("gh workflow run ci.yml")\n'
                'def inspect_python_heredoc(body, safe_marker):',
                1,
            )
            self.assertNotEqual(globals()["PACKET_TEXT"], original)
            with self.assertRaises(AssertionError):
                _scanner_namespace()
            source_assignment = (
                'source = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")'
            )
            source_position = original.rfind(
                source_assignment,
                0,
                original.index("def inspect_python_heredoc(body, safe_marker):"),
            )
            self.assertGreaterEqual(source_position, 0)
            globals()["PACKET_TEXT"] = (
                original[:source_position]
                + 'source = os.system("gh workflow run ci.yml")'
                + original[source_position + len(source_assignment):]
            )
            with self.assertRaises(AssertionError):
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
            'class Settings:\n    os = "darwin"\nprint(Settings.os)\n'
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
            "loop variable rebound": body.replace(
                '    source = Path(name)\n',
                '    name = "experiments/g01-scaleset/.env"\n'
                '    source = Path(name)\n',
                1,
            ),
        }
        for label, source in mutations.items():
            with self.subTest(mutation=label):
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
        for prefix in ('["git", "show", ', '["git", "-P", "show", '):
            with self.subTest(prefix=prefix):
                body = 'import os, subprocess\nwrapper = subprocess.check_output(' + prefix + repr(reference) + '], text=True)\n'
                self.assertIsNotNone(self.inspect(body +
                    'exec(compile(wrapper, "<probe>", "exec"), {"os": os})\n'))
        for prefix in ('["git", "--no-replace-objects", "show", ', '["git", "--no-replace-objects", "-P", "show", '):
            with self.subTest(safe_prefix=prefix):
                body = 'import os, subprocess\nwrapper = subprocess.check_output(' + prefix + repr(reference) + '], text=True)\n'
                self.assertIsNone(self.inspect(body +
                    'exec(compile(wrapper, "<probe>", "exec"), {"os": os})\n'))

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
    unittest.main(verbosity=2)
