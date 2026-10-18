const fs = require('fs');
const assert = require('assert/strict');
const {chromium} = require('playwright');

// Pass the client's redirected log; tokens stay in memory and out of results.
(async () => {
  const log = fs.readFileSync(process.argv[2], 'utf8');
  const match = log.match(/http:\/\/127\.0\.0\.1:8765\/#([a-f0-9]{48})/);
  assert(match, 'Agent URL missing');
  const headers = {Authorization: 'Bearer ' + match[1]};
  const get = async endpoint => {
    const r = await fetch('http://127.0.0.1:8765/api/' + endpoint, {headers});
    assert.equal(r.status, 200); return r.json();
  };
  const browser = await chromium.launch({channel: 'msedge', headless: true});
  try {
    const page = await browser.newPage({viewport: {width: 1180, height: 1050}});
    const errors = []; let requests = 0;
    page.on('pageerror', e => errors.push(e.message));
    page.on('request', r => {if (r.url().endsWith('/api/status')) requests++;});
    await page.goto(match[0]);
    await page.waitForFunction(() => document.getElementById('gpu').textContent !== '--%');
    const modes = [];
    for (let i = 0; i < 6; i++) {
      await page.locator('#centre').click();
      modes.push(await page.locator('#health').textContent());
    }
    assert.equal(modes.slice(0, 5).join('|'), 'PROCESSOR LOAD|GRAPHICS LOAD|MEMORY USAGE|DOWNLOAD / UPLOAD · Mbps|CPU / GPU TEMPERATURES');
    const stacked = await page.evaluate(() => {
      latest={cpu_temp:50,gpu_temp:40};centreMode=5;paintCentre();
      const celsius=el('temperature').textContent;
      el('fahrenheit').checked=true;paintCentre();const fahrenheit=el('temperature').textContent;
      el('fahrenheit').checked=false;latest={gpu_temp:40};paintCentre();const unavailable=el('temperature').textContent;
      centreMode=0;return {celsius,fahrenheit,unavailable};
    });
    assert.deepEqual(stacked,{celsius:'CPU 50°C\nGPU 40°C',fahrenheit:'CPU 122°F\nGPU 104°F',unavailable:'CPU --°C\nGPU 40°C'});
    const lifecycle = await get('lifecycle'); assert(lifecycle.tray);
    // Simulate the standard visibility event deterministically in headless CI.
    await page.evaluate(() => {
      window.testHidden = true;
      Object.defineProperty(document, 'hidden', {get: () => window.testHidden});
      document.dispatchEvent(new Event('visibilitychange'));
    });
    const before = requests;
    const seqBefore = (await get('status')).telemetry.seq;
    await page.waitForTimeout(3200);
    assert.equal(requests, before, 'Hidden dashboard kept polling');
    assert((await get('status')).telemetry.seq >= seqBefore + 3, 'Hidden dashboard stopped device monitoring');
    await page.evaluate(() => {window.testHidden = false; document.dispatchEvent(new Event('visibilitychange'));});
    await page.waitForTimeout(1200);
    assert(requests > before, 'Visible dashboard did not resume');
    const samples = []; let previous;
    for (let i = 0; i < 45; i++) {
      const s = await get('status');
      if (s.telemetry.seq !== previous) {
        samples.push({seq: s.telemetry.seq, at: performance.now()}); previous = s.telemetry.seq;
      }
      assert.equal(s.port, 'COM9'); assert.equal(s.error, '');
      for (const key of ['cpu_load', 'ram_used', 'net_down', 'gpu_temp']) assert.equal(typeof s.telemetry[key], 'number', key);
      await page.waitForTimeout(200);
    }
    const gaps = samples.slice(1).map((s, i) => s.at - samples[i].at);
    assert(Math.max(...gaps) < 1500, 'Slow collectors interrupted the one-second telemetry cadence');
    await page.screenshot({path: '.local/qa/dashboard-optimized.png', fullPage: true});
    assert.deepEqual(errors, []);
    // Intercept Exit to verify browser polling stops without ending this run.
    await page.route('**/api/exit', route => route.fulfill({status: 204}));
    await page.getByRole('button', {name: 'Exit monitor', exact: true}).click();
    const exitRequests = requests; await page.waitForTimeout(2200);
    assert.equal(requests, exitRequests, 'Dashboard kept polling after Exit');
    const result = {modes, stacked, tray: true, hiddenPollingStopped: true, exitPollingStopped: true, telemetryContinuesWhenHidden: true, uniqueSamples: samples.length, maxSampleGapMs: Math.round(Math.max(...gaps)), errors};
    fs.writeFileSync('.local/qa/browser-optimized.json', JSON.stringify(result, null, 2));
    console.log(JSON.stringify(result, null, 2));
  } finally { await browser.close(); }
})().catch(e => {console.error(e); process.exitCode = 1;});
