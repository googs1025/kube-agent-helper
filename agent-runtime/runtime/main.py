"""Entry point for the Agent Pod."""
import os
import sys

from . import logger
from .orchestrator import run_agent
from .reporter import post_findings
from .skill_loader import load_skills


def main() -> None:
    run_id = os.environ.get("RUN_ID")
    if not run_id:
        logger.error("RUN_ID environment variable is not set")
        sys.exit(1)
    skill_names_raw = os.environ.get("SKILL_NAMES", "")
    skill_names = [s.strip() for s in skill_names_raw.split(",") if s.strip()]

    logger.info("agent starting", run_id=run_id, skills=skill_names)

    skills = load_skills(skill_names)
    if not skills:
        logger.error("no skills loaded, exiting")
        sys.exit(1)

    try:
        findings = run_agent(skills)
    except Exception as e:
        import traceback
        logger.error("agent failed", error=str(e), traceback=traceback.format_exc())
        sys.exit(1)
    logger.info("agent completed", findings=len(findings))

    post_findings(run_id, findings)
    logger.info("done")


if __name__ == "__main__":
    main()
