"""Agent Pod 主入口。

整体流程：
    1. 从环境变量读 RUN_ID / SKILL_NAMES（由 Controller 翻译 CR 时注入）
    2. skill_loader 解析 /workspace/skills/*.md（ConfigMap 挂载进来）
    3. orchestrator.run_agent() 跑 LLM + MCP 工具调用循环
    4. reporter.post_findings() 把 findings 回报给 controller HTTP API
    5. 全程 logger 用结构化 JSON 写 stderr，controller 端 collectPodLogs 解析

异常处理：
    - 任何阶段失败都 sys.exit(1)，让 K8s Job 进入 Failed
    - Reconciler 监到 Job.status.Failed 就把 DiagnosticRun 标 Failed 并发通知
"""
import os
import sys

from . import logger
from . import tracer as _tracer
from .orchestrator import run_agent
from .reporter import flush_llm_metrics, post_findings
from .skill_loader import load_skills


def main() -> None:
    run_id = os.environ.get("RUN_ID")
    if not run_id:
        logger.error("RUN_ID environment variable is not set")
        sys.exit(1)
    skill_names_raw = os.environ.get("SKILL_NAMES", "")
    skill_names = [s.strip() for s in skill_names_raw.split(",") if s.strip()]

    logger.info("agent starting", run_id=run_id, skills=skill_names)

    tr = _tracer.init(run_id, skill_names)

    skills = load_skills(skill_names)
    if not skills:
        logger.error("no skills loaded, exiting")
        sys.exit(1)

    try:
        findings = run_agent(skills, tracer=tr)
    except Exception as e:
        import traceback
        logger.error("agent failed", error=str(e), traceback=traceback.format_exc())
        sys.exit(1)
    finally:
        tr.flush()

    logger.info("agent completed", findings=len(findings))

    post_findings(run_id, findings)
    flush_llm_metrics()
    logger.info("done")


if __name__ == "__main__":
    main()
