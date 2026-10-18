const fs=require('fs'),assert=require('assert/strict');
const {chromium}=require('playwright');
(async()=>{
 const log=fs.readFileSync(process.argv[2],'utf8'),match=log.match(/http:\/\/127\.0\.0\.1:8765\/#([a-f0-9]{48})/);assert(match);
 const headers={Authorization:'Bearer '+match[1]};
 const read=async()=>{const r=await fetch('http://127.0.0.1:8765/api/status',{headers});assert.equal(r.status,200);return r.json();};
 const browser=await chromium.launch({channel:'msedge',headless:true});
 try{
  const page=await browser.newPage({viewport:{width:1180,height:1050}}),errors=[];
  page.on('pageerror',e=>errors.push(e.message));await page.goto(match[0]);await page.waitForFunction(()=>document.getElementById('ai').textContent!=='Checking');
  const observed=await read();assert.equal(observed.telemetry.ai_provider,'MAESTRO');assert.equal(observed.telemetry.ai_state,'ACTIVE');
  const labels={ACTIVE:'Active',QUEUED:'Queued',LOADED:'Loaded',WORKER:'Engine running',IDLE:'Idle','NO SOURCE':'No source'};
  for(const [state,label]of Object.entries(labels)){
   await page.evaluate(({state,s})=>paint({...s,telemetry:{ai_state:state,ai_note:'Test activity state'}}),{state,s:observed});
   assert.equal(await page.locator('#ai').textContent(),label);
  }
  await page.evaluate(s=>paint(s),await read());
  assert.equal(await page.locator('#ai').textContent(),'Active');
  assert.match(await page.locator('#model').textContent(),/MAESTRO/);assert.match(await page.locator('#model').textContent(),/Local image\/video generation/);
  assert.deepEqual(errors,[]);
  const life=await fetch('http://127.0.0.1:8765/api/lifecycle',{headers}).then(r=>r.json());assert.equal(life.tray,true);
  await page.screenshot({path:'.local/qa/ai-dashboard.png',fullPage:true});
  const result={liveState:observed.telemetry.ai_state,liveProvider:observed.telemetry.ai_provider,labels,windowsTray:life.tray,errors,preferencesUnchanged:JSON.stringify((await read()).settings)===JSON.stringify(observed.settings)};
  fs.writeFileSync('.local/qa/ai-browser.json',JSON.stringify(result,null,2));console.log(JSON.stringify(result,null,2));
 }finally{await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});