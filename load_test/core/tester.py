"""
核心压测调度器 + 结果聚合
"""
import asyncio
import time
from dataclasses import dataclass, field


@dataclass
class Result:
    scenario: str
    start_time: float = 0
    end_time: float = 0
    total_requests: int = 0
    success_count: int = 0
    error_count: int = 0
    latencies: list = field(default_factory=list)

    def record(self, latency_s: float, success: bool = True):
        if success:
            self.success_count += 1
        else:
            self.error_count += 1
        self.total_requests += 1
        self.latencies.append(latency_s)

    def p50(self):
        return self._percentile(0.50)

    def p95(self):
        return self._percentile(0.95)

    def p99(self):
        return self._percentile(0.99)

    def _percentile(self, q):
        if not self.latencies:
            return 0
        s = sorted(self.latencies)
        idx = int(len(s) * q)
        return s[min(idx, len(s) - 1)] * 1000  # ms

    def error_rate(self):
        if self.total_requests == 0:
            return 0
        return self.error_count / self.total_requests * 100

    def throughput(self):
        elapsed = self.end_time - self.start_time
        if elapsed == 0:
            return 0
        return self.total_requests / elapsed

    def summary(self):
        return f"{self.total_requests} reqs, {self.throughput():.1f} rps, " \
               f"P50={self.p50():.1f}ms P95={self.p95():.1f}ms P99={self.p99():.1f}ms " \
               f"err={self.error_rate():.1f}%"


class Tester:
    def __init__(self, scenario, concurrency: int, duration_sec: int):
        self.scenario = scenario
        self.concurrency = concurrency
        self.duration_sec = duration_sec
        self.result = Result(scenario=scenario.__class__.__name__)
        self._semaphore = asyncio.Semaphore(concurrency)
        self._running = True

    async def run(self) -> Result:
        print(f"  [setup] preparing {self.concurrency} virtual users...")
        await self.scenario.setup(self.concurrency)

        self.result.start_time = time.time()
        print(f"  [run] {self.duration_sec}s with {self.concurrency} concurrent...")

        tasks = []
        for i in range(self.concurrency):
            tasks.append(asyncio.create_task(self._worker(i)))

        await asyncio.sleep(self.duration_sec)
        self._running = False
        await asyncio.gather(*tasks, return_exceptions=True)

        self.result.end_time = time.time()
        print(f"  [teardown] cleaning up...")
        await self.scenario.teardown()

        return self.result

    async def _worker(self, i: int):
        while self._running:
            async with self._semaphore:
                start = time.time()
                try:
                    await self.scenario.execute(i)
                    self.result.record(time.time() - start, success=True)
                except Exception:
                    self.result.record(time.time() - start, success=False)

            await asyncio.sleep(0.01)  # prevent tight loop
