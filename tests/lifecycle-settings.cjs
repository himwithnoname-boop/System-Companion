const fs=require('fs'),assert=require('assert/strict');
const {chromium}=require('playwright');
(async()=>{
 const q='.local/qa/mac-installer-20261018/';
 let match;
 for(let i=0;i<100;i++){match=fs.readFileSync(q+'lifecycle.stderr','utf8').match(/(http:\/\/127\.0\.0\.1:8765)\/#([a-f0-9]{48})/);if(match)break;await new Promise(r=>setTimeout(r,100))}
 assert(match,'Monitor URL unavailable');
 const headers={Authorization:'Bearer '+match[2],'Content-Type':'application/json'};
 const api=async(name,body)=>{const res=await fetch(match[1]+'/api/'+name,{headers,method:body===undefined?'GET':'POST',body:body===undefined?undefined:JSON.stringify(body)});if(!res.ok)throw Error(await res.text());return res.status===204?null:res.json()};
 let state;
 for(let i=0;i<40;i++){state=await api('status');if(state.port&&state.settings)break;await new Promise(r=>setTimeout(r,250))}
 assert(state.port&&state.settings);
 const original=state.settings;
 const restore=Object.fromEntries(['theme','brightness','fahrenheit','auto_rotate','center_mode','timeout','page','gpu_hidden'].map(k=>[k,original[k]]));
 const browser=await chromium.launch({channel:'msedge',headless:true});
 const errors=[];let exitRequests=0;
 try{
  const a=await browser.newPage(),b=await browser.newPage();
  for(const page of [a,b]){
   page.on('pageerror',e=>errors.push(e.message));page.on('request',r=>{if(r.url().endsWith('/api/exit'))exitRequests++});
   await page.goto(match[0]);await page.waitForFunction(()=>!document.getElementById('apply').disabled);
   await page.evaluate(()=>{Object.defineProperty(document,'hidden',{get:()=>false});document.dispatchEvent(new Event('visibilitychange'))});
  }
  const theme=(original.theme+1)%5;
  await a.locator('#theme').selectOption(String(theme));await a.locator('#apply').click();
  await b.waitForFunction(value=>document.getElementById('theme').value===String(value),theme);
  await b.locator('#fahrenheit').setChecked(!original.fahrenheit);await b.locator('#apply').click();
  await a.waitForFunction(value=>document.getElementById('fahrenheit').checked===value,!original.fahrenheit);
  const draft=original.brightness===130?140:130;
  await a.locator('#brightness').evaluate((e,value)=>{e.value=value;e.dispatchEvent(new Event('input',{bubbles:true}))},draft);
  const theme2=(theme+1)%5;
  await b.locator('#theme').selectOption(String(theme2));await b.locator('#apply').click();
  await a.waitForFunction(value=>document.getElementById('theme').value===String(value),theme2);
  assert.equal(await a.locator('#brightness').inputValue(),String(draft),'External update overwrote an unapplied draft');
  const prior=(await api('status')).telemetry.seq;
  await a.close();await b.close();await browser.close();
  await new Promise(r=>setTimeout(r,2300));
  const after=await api('status');
  assert(after.telemetry.seq>prior,'Closing dashboard stopped telemetry');
  assert(after.port&&!after.error);
  assert.equal((await api('lifecycle')).tray,true);
  assert.equal(exitRequests,0,'Closing dashboard requested host shutdown');
  assert.deepEqual(errors,[]);
  const receipt={dashboardCloseLeavesHostAndTrayRunning:true,telemetryContinuesAfterBrowserExit:true,settingsSyncAcrossDashboardsAndDevice:true,unappliedDraftPreserved:true,closingPageSendsNoExitRequest:true,errors};
  fs.writeFileSync(q+'lifecycle-settings.json',JSON.stringify(receipt,null,2));console.log(JSON.stringify(receipt));
 }finally{
  await api('settings',restore);
  await browser.close();
  // The test explicitly exits its temporary host; browser closure above did not.
  await api('exit',{});
 }
})().catch(e=>{console.error(e);process.exitCode=1});
