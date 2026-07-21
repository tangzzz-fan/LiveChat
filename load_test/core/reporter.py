"""
报告生成器：Markdown + JSON 输出
"""
import time
from datetime import datetime
from collections import OrderedDict


class Reporter:
    def __init__(self, results: dict, args: dict):
        self.results = results
        self.args = args

    def to_markdown(self) -> str:
        lines = []
        lines.append("# LiveChat Load Test Report")
        lines.append("")
        lines.append(f"**Date:** {datetime.now().isoformat()}")
        lines.append(f"**Concurrency:** {self.args.get('concurrency')}")
        lines.append(f"**Duration:** {self.args.get('duration')}s")
        lines.append(f"**Base URL:** {self.args.get('base_url')}")
        lines.append("")

        lines.append("## Summary")
        lines.append("")
        lines.append("| Scenario | Requests | Throughput | P50 | P95 | P99 | Errors |")
        lines.append("|----------|----------|------------|-----|-----|-----|--------|")
        for name, r in self.results.items():
            lines.append(
                f"| {name} | {r.total_requests} | {r.throughput():.1f} req/s | "
                f"{r.p50():.1f}ms | {r.p95():.1f}ms | {r.p99():.1f}ms | "
                f"{r.error_rate():.1f}% |"
            )
        lines.append("")

        for name, r in self.results.items():
            lines.append(f"## {name}")
            lines.append("")
            lines.append(f"- **Total requests:** {r.total_requests}")
            lines.append(f"- **Success:** {r.success_count} ({100 - r.error_rate():.1f}%)")
            lines.append(f"- **Errors:** {r.error_count} ({r.error_rate():.1f}%)")
            lines.append(f"- **Throughput:** {r.throughput():.1f} req/s")
            elapsed = r.end_time - r.start_time
            lines.append(f"- **Elapsed:** {elapsed:.1f}s")
            lines.append(f"- **P50 latency:** {r.p50():.1f}ms")
            lines.append(f"- **P95 latency:** {r.p95():.1f}ms")
            lines.append(f"- **P99 latency:** {r.p99():.1f}ms")
            lines.append("")

        return "\n".join(lines)

    def to_json(self) -> dict:
        out = {
            "meta": {
                "date": datetime.now().isoformat(),
                "concurrency": self.args.get("concurrency"),
                "duration": self.args.get("duration"),
            },
            "scenarios": {},
        }
        for name, r in self.results.items():
            out["scenarios"][name] = {
                "total_requests": r.total_requests,
                "success_count": r.success_count,
                "error_count": r.error_count,
                "error_rate_pct": r.error_rate(),
                "throughput_rps": r.throughput(),
                "p50_ms": r.p50(),
                "p95_ms": r.p95(),
                "p99_ms": r.p99(),
            }
        return out
