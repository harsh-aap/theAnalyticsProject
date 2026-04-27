import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('errors');
const ingestLatency = new Trend('ingest_latency', true);

// Target: 10k req/s with p99 < 10ms
// Run stages: ramp up → sustain → push → ramp down
export const options = {
  stages: [
    { duration: '30s', target: 500 },    // warm up
    { duration: '1m',  target: 2000 },   // medium load
    { duration: '2m',  target: 5000 },   // high load
    { duration: '2m',  target: 10000 },  // target load
    { duration: '30s', target: 0 },      // ramp down
  ],
  thresholds: {
    http_req_duration:        ['p(99)<10'],   // 10ms p99 — the SLO
    http_req_duration:        ['p(95)<7'],    // 7ms p95
    errors:                   ['rate<0.01'],  // <1% error rate
    http_req_failed:          ['rate<0.01'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const API_KEY  = __ENV.API_KEY  || '';

const EVENT_TYPES = ['page_view', 'click', 'add_to_cart', 'purchase', 'search'];
const SOURCES = ['web', 'ios', 'android'];

function randomEvent() {
  return {
    event_type: EVENT_TYPES[Math.floor(Math.random() * EVENT_TYPES.length)],
    user_id:    `user-${Math.floor(Math.random() * 10000)}`,
    session_id: `sess-${Math.floor(Math.random() * 50000)}`,
    product_id: `prod-${Math.floor(Math.random() * 1000)}`,
    price:      Math.random() < 0.3 ? parseFloat((Math.random() * 200).toFixed(2)) : 0,
    source:     SOURCES[Math.floor(Math.random() * SOURCES.length)],
  };
}

// Default scenario: single-event endpoint
export default function () {
  const payload = JSON.stringify(randomEvent());
  const params  = { headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${API_KEY}` } };

  const res = http.post(`${BASE_URL}/v1/events`, payload, params);

  const ok = check(res, {
    'status is 202': (r) => r.status === 202,
    'has event_id':  (r) => JSON.parse(r.body).event_id !== undefined,
  });

  errorRate.add(!ok);
  ingestLatency.add(res.timings.duration);
}

// Batch scenario — run with: k6 run -e SCENARIO=batch -e API_KEY=xxx -e BASE_URL=http://localhost:8080 load_test.js
export function batch() {
  const events  = Array.from({ length: 50 }, randomEvent);
  const payload = JSON.stringify(events);
  const params  = { headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${API_KEY}` } };

  const res = http.post(`${BASE_URL}/v1/events/batch`, payload, params);

  const ok = check(res, {
    'status is 202': (r) => r.status === 202,
  });

  errorRate.add(!ok);
}
