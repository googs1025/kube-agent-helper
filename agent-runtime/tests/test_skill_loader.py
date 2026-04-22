"""Tests for runtime.skill_loader."""
import os
import tempfile
from runtime.skill_loader import load_skills, _parse_skill_md, Skill


def _write_skill(tmp_dir: str, name: str, content: str) -> str:
    path = os.path.join(tmp_dir, f"{name}.md")
    with open(path, "w") as f:
        f.write(content)
    return path


VALID_SKILL_MD = """\
---
name: pod-health-analyst
dimension: health
tools: ["kubectl_get", "kubectl_describe"]
requires_data: ["pods"]
---
Check pod health and report issues.
"""

MINIMAL_SKILL_MD = """\
---
name: minimal
tools: ["kubectl_get"]
---
Minimal prompt.
"""


class TestParseSkillMd:
    def test_valid_skill(self, tmp_path):
        path = _write_skill(str(tmp_path), "test", VALID_SKILL_MD)
        skill = _parse_skill_md(path)
        assert skill is not None
        assert skill.name == "pod-health-analyst"
        assert skill.dimension == "health"
        assert skill.tools == ["kubectl_get", "kubectl_describe"]
        assert skill.requires_data == ["pods"]
        assert "Check pod health" in skill.prompt

    def test_minimal_skill_defaults(self, tmp_path):
        path = _write_skill(str(tmp_path), "min", MINIMAL_SKILL_MD)
        skill = _parse_skill_md(path)
        assert skill is not None
        assert skill.name == "minimal"
        assert skill.dimension == "health"  # default
        assert skill.requires_data == []

    def test_no_frontmatter_returns_none(self, tmp_path):
        path = _write_skill(str(tmp_path), "bad", "no frontmatter here")
        assert _parse_skill_md(path) is None

    def test_tools_as_string(self, tmp_path):
        md = '---\nname: t\ntools: \'["a","b"]\'\n---\nprompt'
        path = _write_skill(str(tmp_path), "str_tools", md)
        skill = _parse_skill_md(path)
        assert skill is not None
        assert skill.tools == ["a", "b"]

    def test_tools_as_list(self, tmp_path):
        md = "---\nname: t\ntools:\n  - x\n  - y\n---\nprompt"
        path = _write_skill(str(tmp_path), "list_tools", md)
        skill = _parse_skill_md(path)
        assert skill is not None
        assert skill.tools == ["x", "y"]


class TestLoadSkills:
    def test_loads_requested_skills(self, tmp_path, monkeypatch):
        monkeypatch.setattr("runtime.skill_loader.SKILLS_DIR", str(tmp_path))
        _write_skill(str(tmp_path), "alpha", VALID_SKILL_MD)
        _write_skill(str(tmp_path), "beta", MINIMAL_SKILL_MD)

        skills = load_skills(["alpha", "beta"])
        assert len(skills) == 2

    def test_missing_skill_skipped(self, tmp_path, monkeypatch):
        monkeypatch.setattr("runtime.skill_loader.SKILLS_DIR", str(tmp_path))
        _write_skill(str(tmp_path), "exists", VALID_SKILL_MD)

        skills = load_skills(["exists", "missing"])
        assert len(skills) == 1
        assert skills[0].name == "pod-health-analyst"

    def test_empty_list(self, tmp_path, monkeypatch):
        monkeypatch.setattr("runtime.skill_loader.SKILLS_DIR", str(tmp_path))
        assert load_skills([]) == []
