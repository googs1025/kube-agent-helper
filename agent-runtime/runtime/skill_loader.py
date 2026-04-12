"""Loads SKILL.md files from /workspace/skills/"""
import os
import re
from dataclasses import dataclass, field
from typing import List

SKILLS_DIR = os.environ.get("SKILLS_DIR", "/workspace/skills")


@dataclass
class Skill:
    name: str
    dimension: str
    tools: List[str]
    prompt: str
    requires_data: List[str] = field(default_factory=list)


def load_skills(skill_names: List[str]) -> List[Skill]:
    """Load only the requested skills from /workspace/skills/<name>.md"""
    skills = []
    for name in skill_names:
        path = os.path.join(SKILLS_DIR, f"{name}.md")
        if not os.path.exists(path):
            print(f"[warn] skill file not found: {path}")
            continue
        skill = _parse_skill_md(path)
        if skill:
            skills.append(skill)
    return skills


def _parse_skill_md(path: str) -> "Skill | None":
    with open(path) as f:
        content = f.read()

    # Extract YAML frontmatter between --- markers
    match = re.match(r"^---\n(.*?)\n---\n(.*)", content, re.DOTALL)
    if not match:
        return None

    import yaml
    import json
    meta = yaml.safe_load(match.group(1))
    prompt_body = match.group(2).strip()

    tools_raw = meta.get("tools", "[]")
    if isinstance(tools_raw, str):
        tools = json.loads(tools_raw)
    else:
        tools = tools_raw

    return Skill(
        name=meta["name"],
        dimension=meta.get("dimension", "health"),
        tools=tools,
        prompt=prompt_body,
        requires_data=meta.get("requires_data", []),
    )
