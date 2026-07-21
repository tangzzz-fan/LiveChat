"""
LiveChat 压测框架 — 主入口
"""
import asyncio
import argparse
import json
import time
from datetime import datetime
from core.tester import Tester
from core.reporter import Reporter


SCENARIOS = {}

def _load():
    try:
        from scenarios.send_message import SendMessageScenario
        SCENARIOS["send_message"] = SendMessageScenario
    except ImportError:
        pass
    try:
        from scenarios.connect import ConnectScenario
        SCENARIOS["connect"] = ConnectScenario
    except ImportError:
        pass
    try:
        from scenarios.group_fanout import GroupFanoutScenario
        SCENARIOS["group_fanout"] = GroupFanoutScenario
    except ImportError:
        pass
    try:
        from scenarios.sync_backfill import SyncBackfillScenario
        SCENARIOS["sync_backfill"] = SyncBackfillScenario
    except ImportError:
        pass
    try:
        from scenarios.reconnect_storm import ReconnectStormScenario
        SCENARIOS["reconnect_storm"] = ReconnectStormScenario
    except ImportError:
        pass

_load()


def parse_args():
    parser = argparse.ArgumentParser(description="LiveChat Load Tester")
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--ws-url", default="ws://localhost:8081/ws")
    parser.add_argument("--concurrency", type=int, default=100)
    parser.add_argument("--duration", type=int, default=60, help="Duration in seconds")
    parser.add_argument("--scenario", choices=list(SCENARIOS.keys()), help="Scenario to run")
    parser.add_argument("--all", action="store_true", help="Run all scenarios")
    parser.add_argument("--output", default="markdown", choices=["markdown", "json"])
    parser.add_argument("--jitter-ms", type=int, default=500)
    parser.add_argument("--no-jitter", action="store_true")
    parser.add_argument("--quick", action="store_true", help="Quick mode: 10 concurrency, 10s")
    return parser.parse_args()


async def run_scenario(name, args):
    cls = SCENARIOS[name]
    if name == "reconnect_storm":
        scenario = cls(args.base_url, args.ws_url, jitter_ms=args.jitter_ms, no_jitter=args.no_jitter)
    else:
        scenario = cls(args.base_url, args.ws_url)
    concurrency = 10 if args.quick else args.concurrency
    duration = 10 if args.quick else args.duration

    tester = Tester(scenario, concurrency, duration)
    result = await tester.run()

    print(f"\n  {name}: {result.summary()}")
    return name, result


async def main():
    args = parse_args()

    if args.quick:
        args.concurrency = 10
        args.duration = 10

    scenarios_to_run = list(SCENARIOS.keys()) if args.all else [args.scenario] if args.scenario else []

    if not scenarios_to_run:
        print("Available scenarios:", ", ".join(SCENARIOS.keys()))
        print("Use --scenario <name> or --all")
        return

    print(f"=== LiveChat Load Test ===")
    print(f"Time: {datetime.now().isoformat()}")
    print(f"Scenarios: {', '.join(scenarios_to_run)}")
    print(f"Concurrency: {args.concurrency}, Duration: {args.duration}s")
    print(f"Base URL: {args.base_url}")
    print()

    results = {}
    for name in scenarios_to_run:
        _, result = await run_scenario(name, args)
        results[name] = result

    reporter = Reporter(results, vars(args))

    if args.output == "json":
        print("\n" + json.dumps(reporter.to_json(), indent=2))
    else:
        md = reporter.to_markdown()
        print("\n" + md)

        # Save baseline
        import os
        os.makedirs("baselines", exist_ok=True)
        ts = datetime.now().strftime("%Y%m%d-%H%M%S")
        path = f"baselines/baseline-{ts}.md"
        with open(path, "w") as f:
            f.write(md)
        print(f"\nBaseline saved to {path}")


if __name__ == "__main__":
    asyncio.run(main())
