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
import { check } from 'k6';
import { Counter } from 'k6/metrics';
import exec from 'k6/execution';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:80';
const API = `${BASE_URL}/api`;

const TEST_CODE = '123456';

// 51 个用户，每个 VU 固定使用一个用户
const PHONES = [
  '13800138000',
  ...Array.from({ length: 50 }, (_, i) => `138001380${String(i + 1).padStart(2, '0')}`),
];

// 13 个秒杀券（6-18）
const VOUCHER_IDS = [6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18];

// 自定义指标：成功抢购（200，会走 RocketMQ 异步下单）
const seckillSuccess = new Counter('seckill_success');
const seckillRateLimited = new Counter('seckill_rate_limited');
const seckillAlreadyBought = new Counter('seckill_already_bought');

export const options = {
  scenarios: {
    seckill: {
      executor: 'constant-arrival-rate',
      rate: 100,
      timeUnit: '1s',
      duration: '60s',
      preAllocatedVUs: 30,
      maxVUs: 51,
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<3000'],
  },
};

let cachedToken = null;
function getToken(phone) {
  if (cachedToken) return cachedToken;
  const res = http.post(
    `${API}/user/login`,
    JSON.stringify({ phone, code: TEST_CODE }),
    { headers: { 'Content-Type': 'application/json' } }
  );
  if (res.status !== 200) return null;
  const body = JSON.parse(res.body);
  if (body.success && body.data) {
    cachedToken = body.data;
    return cachedToken;
  }
  return null;
}

function seckill() {
  const vuId = exec.vu.idInTest;
  const phone = PHONES[(vuId - 1) % PHONES.length];
  const token = getToken(phone);
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
}

export default seckill;
export { seckill };
