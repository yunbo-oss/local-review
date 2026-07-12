/**
 * 秒杀全链路压测：每人抢到 1 张即停止，无无效重复请求
 * 目标：测量「从开始到所有人抢完」的真实耗时与有效 QPS
 * 前置：make seed && make seed-load-test && make seed-redis，服务已启动
 *
 * 用法:
 *   k6 run script/load-test-seckill-e2e.js
 *   k6 run -e BASE_URL=http://localhost:80 script/load-test-seckill-e2e.js
 */
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';
import exec from 'k6/execution';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:80';
const API = `${BASE_URL}/api`;

const TEST_CODE = '123456';

// 151 个用户
const PHONES = [
  '13800138000',
  ...Array.from({ length: 50 }, (_, i) => `138001380${String(i + 1).padStart(2, '0')}`),
  ...Array.from({ length: 100 }, (_, i) => `13800138${String(i + 51).padStart(3, '0')}`),
];

// 25 个秒杀券（6-30）
const VOUCHER_IDS = [6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30];

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

const seckillSuccess = new Counter('seckill_success');
const seckillRateLimited = new Counter('seckill_rate_limited');
const seckillAlreadyBought = new Counter('seckill_already_bought');
const seckillOther = new Counter('seckill_other'); // 404/500/连接错误等

const VUS = PHONES.length;
// 每人最多尝试次数（库存充足时通常几次即成功）
const MAX_TRIES_PER_VU = 200;

export const options = {
  setupTimeout: '120s',
  scenarios: {
    seckill_e2e: {
      executor: 'per-vu-iterations',
      vus: VUS,
      iterations: MAX_TRIES_PER_VU,
      maxDuration: '180s',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<3000'],
    seckill_success: ['count>=' + VUS],
  },
};

// 每个 VU 独立状态：抢到即不再发请求（k6 每个 VU 有独立模块副本）
let vuGrabbed = false;

function seckill(data) {
  if (vuGrabbed) return;

  const vuId = exec.vu.idInTest;
  const token = data.tokens[vuId - 1];
  if (!token) return;

  const iter = exec.vu.iterationInScenario;
  const voucherIdx = (vuId - 1 + iter) % VOUCHER_IDS.length;
  const voucherId = VOUCHER_IDS[voucherIdx];

  const res = http.post(
    `${API}/voucher-order/seckill/${voucherId}`,
    null,
    { headers: { authorization: token } }
  );

  if (res.status === 200) {
    seckillSuccess.add(1);
    vuGrabbed = true;
  } else if (res.status === 429) {
    seckillRateLimited.add(1);
    sleep(0.05);
  } else if (res.status === 400) {
    seckillAlreadyBought.add(1);
    vuGrabbed = true;
  } else {
    seckillOther.add(1);
    if (res.status === 404) sleep(0.1); // 404 可能是布隆过滤器未加载券 9-30，稍后重试
  }

  check(res, { 'seckill responded': (r) => r.status > 0 });
}

export default seckill;
export { seckill };
