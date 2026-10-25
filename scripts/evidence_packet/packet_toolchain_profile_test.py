"""Static selected-profile and frozen v0.4 packet identity checks.

Inert-text checks only. The packet markdown and branch pin files are read as
data; one wrapper assignment line is inspected with ``ast.parse`` (parse-only,
never exec/compile/eval/import). Packet recipes, specimens, and audit blocks
are never executed, compiled, evaluated, or imported. No subprocesses.

Trust boundary: stdlib only (unittest/re/ast/pathlib/hashlib), file reads under the
repository root, no network, no shell, no Git, no packet execution.
"""

from __future__ import annotations

import ast
import hashlib
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
PACKET_PATH = ROOT / "docs" / "evidence" / "g01-recovery-packet.md"
GO_MOD_PATH = ROOT / "go.mod"
CHECK_TOOLCHAIN_PATH = ROOT / "scripts" / "check-toolchain.sh"
ACTIVE_GO = "1.27.3"
FROZEN_GO = "1.26.9"
FROZEN_PACKET_SHA256 = "e2a0d876e47e9ced7565a5dbde083f72ae1aeb14a7ac01cc9fccf56abf377959"


def branch_go_directive() -> str:
    text = GO_MOD_PATH.read_text(encoding="utf-8")
    match = re.search(r"^go (\d+\.\d+\.\d+)$", text, re.MULTILINE)
    assert match is not None, "root go.mod directive not found"
    return match.group(1)


def branch_expected_toolchain() -> str:
    text = CHECK_TOOLCHAIN_PATH.read_text(encoding="utf-8")
    match = re.search(r'^expected_toolchain="(go\d+\.\d+\.\d+)"$', text, re.MULTILINE)
    assert match is not None, "check-toolchain.sh expected_toolchain not found"
    return match.group(1)


def wrapper_fence_text(packet: str) -> str:
    anchor = packet.index('reviewed_toolchain = "go')
    fence_start = packet.rfind("```sh\n", 0, anchor)
    if fence_start < 0:
        raise AssertionError("packet wrapper fence start not found")
    fence_end = packet.index("\n```\n", anchor)
    return packet[fence_start:fence_end]


def reviewed_toolchain_assignment(packet: str) -> str:
    match = re.search(r'^reviewed_toolchain = "go\d+\.\d+\.\d+"$', packet, re.MULTILINE)
    assert match is not None, "reviewed_toolchain assignment not found"
    return match.group(0)


class PacketToolchainProfileTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.packet = PACKET_PATH.read_text(encoding="utf-8")
        cls.branch_directive = branch_go_directive()
        cls.branch_toolchain = branch_expected_toolchain()
        assert cls.branch_toolchain == "go" + cls.branch_directive, (
            f"branch pins disagree: go.mod {cls.branch_directive} vs "
            f"check-toolchain.sh {cls.branch_toolchain}"
        )

    def test_frozen_packet_identity(self) -> None:
        # The v0.4 packet is a versioned evidence artifact, not a template to
        # relabel v0.5. Any byte change invalidates this frozen identity.
        self.assertEqual(hashlib.sha256(PACKET_PATH.read_bytes()).hexdigest(), FROZEN_PACKET_SHA256)

    def test_active_module_pins_agree(self) -> None:
        self.assertEqual(self.branch_directive, ACTIVE_GO)
        for relative in ("go.mod", "experiments/g01-scaleset/go.mod", "experiments/g02-auth/go.mod", "experiments/r1-credentials/go.mod"):
            text = (ROOT / relative).read_text(encoding="utf-8")
            self.assertRegex(text, rf"(?m)^go {re.escape(ACTIVE_GO)}$")
            self.assertNotRegex(text, r"(?m)^toolchain ")

    def test_active_sdk_pin(self) -> None:
        text = (ROOT / "experiments/g01-scaleset/go.mod").read_text(encoding="utf-8")
        self.assertRegex(text, r"(?m)^\s+github\.com/actions/scaleset v0\.5\.0$")

    def test_pins_table_matches_frozen_profile(self) -> None:
        match = re.search(
            r"\| Go toolchain \| Current experiment module directive `go (\d+\.\d+\.\d+)`; "
            r"verification toolchain `GOTOOLCHAIN=(go\d+\.\d+\.\d+)`",
            self.packet,
        )
        self.assertIsNotNone(match, "current pins-table Go toolchain row not found")
        assert match is not None
        self.assertEqual(match.group(1), FROZEN_GO)
        self.assertEqual(match.group(2), "go" + FROZEN_GO)

    def test_wrapper_reviewed_toolchain_matches_frozen_profile(self) -> None:
        line = reviewed_toolchain_assignment(self.packet)
        module = ast.parse(line)
        self.assertEqual(len(module.body), 1)
        statement = module.body[0]
        self.assertIsInstance(statement, ast.Assign)
        assert isinstance(statement, ast.Assign)
        self.assertEqual(len(statement.targets), 1)
        target = statement.targets[0]
        self.assertIsInstance(target, ast.Name)
        assert isinstance(target, ast.Name)
        self.assertEqual(target.id, "reviewed_toolchain")
        value = statement.value
        self.assertIsInstance(value, ast.Constant)
        assert isinstance(value, ast.Constant)
        self.assertEqual(value.value, "go" + FROZEN_GO)

    def test_prescription_toolchains_match_frozen_profile(self) -> None:
        fence = wrapper_fence_text(self.packet)
        versions = re.findall(r"go(\d+\.\d+\.\d+)", fence)
        self.assertGreater(len(versions), 0, "no toolchain versions in wrapper fence")
        for version in versions:
            self.assertEqual(
                "go" + version,
                "go" + FROZEN_GO,
                "wrapper-fence toolchain differs from frozen v0.4 profile",
            )

    def test_historical_toolchain_records_preserved(self) -> None:
        fence = wrapper_fence_text(self.packet)
        outside = self.packet.replace(fence, "")
        historical = re.findall(r"go1\.26\.8", outside)
        self.assertGreater(
            len(historical), 0, "historical go1.26.8 records were removed"
        )
        current_old = re.findall(r"go1\.26\.8", fence)
        self.assertEqual(
            current_old,
            [],
            "stale go1.26.8 remains in the current-profile wrapper fence",
        )


if __name__ == "__main__":
    unittest.main()
