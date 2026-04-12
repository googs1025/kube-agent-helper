"""Entry point for the Agent Pod."""
import os
import sys

from .orchestrator import run_agent
from .reporter import post_findings
from .skill_loader import load_skills


def main() -> None:
    run_id = os.environ["RUN_ID"]
    skill_names_raw = os.environ.get("SKILL_NAMES", "")
    skill_names = [s.strip() for s in skill_names_raw.split(",") if s.strip()]

    print(f"[info] run_id={run_id} skills={skill_names}")

    skills = load_skills(skill_names)
    if not skills:
        print("[error] no skills loaded — exiting")
        sys.exit(1)

    findings = run_agent(skills)
    print(f"[info] found {len(findings)} findings")

    post_findings(run_id, findings)
    print("[info] done")


if __name__ == "__main__":
    main()
