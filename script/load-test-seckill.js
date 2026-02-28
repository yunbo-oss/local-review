/**
 * 秒杀专用压测脚本（多用户 + 多秒杀券）
 * 目标：控制 QPS 在限流内，让更多请求成功，走完整消息队列逻辑
 * 前置：make seed && make seed-load-test && make seed-redis，服务已启动
 *
 * 用法:
 *   k6 run script/load-test-seckill.js
 *   k6 run -e BASE_URL=http://localhost:80 script/load-test-seckill.js
 */
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';
import exec from 'k6/execution';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:80';
const API = `${BASE_URL}/api`;

const TEST_CODE = '123456';

// 51 个用户
const PHONES = [
  '13800138000',
  ...Array.from({ length: 50 }, (_, i) => `138001380${String(i + 1).padStart(2, '0')}`),
];

// 13 个秒杀券（6-18）
const VOUCHER_IDS = [6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18];

// 压测前预先登录 51 用户（登录限流已移除，可快速登录）
export function setup() {
  const BASE_URL = __ENV.BASE_URL || 'http://localhost:80';
  const API = `${BASE_URL}/api`;
  const tokens = [];
  for (let i = 0; i < PHONES.length; i++) {
    const res = http.post(
      `${API}/user/login`,
      JSON.stringify({ phone: PHONES[i], code: TEST_CODE }),
      { headers: { 'Content-Type': 'application/json' } }
    );
    if (res.status === 200) {
      const body = JSON.parse(res.body);
      tokens.push(body.success && body.data ? body.data : null);
    } else {
      tokens.push(null);
    }
    if (i < PHONES.length - 1) sleep(0.1);
  }
  const count = tokens.filter(Boolean).length;
  console.log(`[setup] ${count}/${PHONES.length} 用户登录成功`);
  return { tokens };
}

// 自定义指标：成功抢购（200，会走 RocketMQ 异步下单）
const seckillSuccess = new Counter('seckill_success');
const seckillRateLimited = new Counter('seckill_rate_limited');
const seckillAlreadyBought = new Counter('seckill_already_bought');

export const options = {
  setupTimeout: '90s',
  scenarios: {
    seckill: {
      // 必须 51 VU 同时跑，每 VU 一用户，否则 constant-arrival-rate 只起 1-3 个 VU
      executor: 'per-vu-iterations',
      vus: 51,
      iterations: 200,
      maxDuration: '120s',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<3000'],
  },
};

function seckill(data) {
  const vuId = exec.vu.idInTest;
  const token = data.tokens[vuId - 1];
  if (!token) return;

  // 每个 VU 轮询不同券，分散到 13 个券上，避免都抢同一张
  const iter = exec.vu.iterationInScenario;
  const voucherIdx = (vuId - 1 + iter) % VOUCHER_IDS.length;
  const voucherId = VOUCHER_IDS[voucherIdx];

  const res = http.post(
    `${API}/voucher-order/seckill/${voucherId}`,
    null,
    { headers: { authorization: token } }
  );

  if (res.status === 200) seckillSuccess.add(1);
  else if (res.status === 429) seckillRateLimited.add(1);
  else if (res.status === 400) seckillAlreadyBought.add(1);

  check(res, { 'seckill responded': (r) => r.status > 0 });

  // 控制 QPS：51 VU × 2.5 req/s ≈ 127，低于限流 150
  sleep(0.4);
}

export default seckill;
export { seckill };
