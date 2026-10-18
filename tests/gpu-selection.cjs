const fs=require('fs'),assert=require('assert/strict'),http=require('http'),vm=require('vm');
const {chromium}=require('playwright');
const html=fs.readFileSync('host/dashboard.html','utf8');new vm.Script(html.match(/<script>([\s\S]*)<\/script>/)[1]);
(async()=>{
 const q='.local/qa/',token='1'.repeat(48);
 const gpus=Array.from({length:4},(_,i)=>({index:i,id:'fixture-'+i,display_key:String(i).padStart(32,'0'),name:i===0?'<img src=x onerror=alert(1)>':'Fixture GPU '+i,temp:40+i,load:[100,0,20,undefined][i],vram:1,vram_total:16,power:30}));
 const state={version:'1.0.8',port:'Fixture',error:'',settings:{theme:0,brightness:170,fahrenheit:false,page:0,auto_rotate:true,center_mode:5,timeout:0,gpu_selection:true,host_key:'a'.repeat(32),gpu_hidden:[]},telemetry:{seq:1,cpu_temp:50,cpu_load:10,ram_used:8,ram_total:16,gpu_count:4,gpu_load:40,gpu_load_available:3,gpu_load_method:'mean_available',gpus}};
 let posts=[];
 const server=http.createServer((req,res)=>{
  const json=x=>{res.setHeader('Content-Type','application/json');res.end(JSON.stringify(x))};
  if(req.url==='/'){res.setHeader('Content-Type','text/html');res.end(html);return}
  if(req.url==='/api/status'){state.telemetry.seq++;json(state);return}
  if(req.url==='/api/settings'&&req.method==='POST'){let body='';req.on('data',d=>body+=d);req.on('end',()=>{const changes=JSON.parse(body);posts.push(changes);Object.assign(state.settings,changes);res.writeHead(204);res.end()});return}
  if(req.url==='/api/settings'){json(state.settings);return}
  if(req.url==='/api/sensor-access'){json({available:false});return}
  json({tray:true,available:false});
 });await new Promise(resolve=>server.listen(8770,'127.0.0.1',resolve));
 const browser=await chromium.launch({channel:'msedge',headless:true});const errors=[];
 try{
  const page=await browser.newPage({viewport:{width:1180,height:1050}});page.on('pageerror',e=>errors.push(e.message));
  await page.goto('http://127.0.0.1:8770/#'+token);await page.waitForSelector('.gpu-choice input');
  assert.equal(await page.locator('.gpu-card').count(),4);assert.equal(await page.locator('.gpu-choice input').count(),4);
  assert.equal(await page.locator('.gpu-card img').count(),0);assert.equal(await page.locator('.gpu-choice img').count(),0);
  assert.deepEqual(await page.locator('.gpu-card').first().locator('.gpu-metrics span').evaluateAll(es=>es.map(e=>e.firstChild.textContent)),['Temperature','Load','Memory','Power']);
  const bar=()=>page.locator('#gpuRing').evaluate(e=>e.style.getPropertyValue('--value'));
  assert.equal(await bar(),'40');
  await page.locator('.gpu-choice input').nth(0).uncheck();assert.equal(await page.locator('.gpu-card').count(),3);assert.equal(posts.length,0);assert.equal(await bar(),'10');
  await page.waitForTimeout(1500);assert.equal(await page.locator('.gpu-choice input').nth(0).isChecked(),false,'Unsaved choice lost while polling');
  await page.locator('#apply').click();await page.waitForFunction(()=>document.getElementById('feedback').textContent.includes('sent'));
  assert.deepEqual(posts[0].gpu_hidden,[gpus[0].display_key]);await page.goto('http://127.0.0.1:8770/#'+token);await page.waitForSelector('.gpu-choice input');assert.equal(await page.locator('.gpu-choice input').nth(0).isChecked(),false);
  assert.match(await page.locator('.gpu-card h3').first().textContent(),/^GPU1/);assert.equal(await bar(),'10');
  await page.screenshot({path:q+'dashboard-gpu-selection.png',fullPage:true});
  for(let i=1;i<4;i++)await page.locator('.gpu-choice input').nth(i).uncheck();
  assert.equal(await page.locator('.gpu-card').count(),0);assert.equal(await bar(),'0');assert.equal(await page.locator('#gpu').textContent(),'--%');
  await page.evaluate(()=>{centreMode=2;paintCentre()});assert.equal(await page.locator('#health').textContent(),'NO GPUS SELECTED');
  await page.locator('.gpu-choice input').nth(2).check();await page.evaluate(()=>{centreMode=5;paintCentre()});
  assert.equal(await page.locator('#temperature').textContent(),'CPU 50°C\nGPU2 42°C');
  await page.setViewportSize({width:390,height:844});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);
  await page.screenshot({path:q+'dashboard-gpu-selection-mobile.png',fullPage:true});
  await page.locator('.gpu-choice input').nth(2).uncheck();
  await page.locator('.gpu-choice input').nth(0).check();assert.equal(await bar(),'100');
  await page.locator('.gpu-choice input').nth(0).uncheck();
  await page.locator('.gpu-choice input').nth(3).check();assert.equal(await bar(),'0');assert.equal(await page.locator('#gpu').textContent(),'--%');
  assert.deepEqual(errors,[]);
  const result={fourSummariesOnly:true,zeroBasedIndices:true,safeNames:true,selectionSurvivedPollingAndReopen:true,selectedGPUBarUpdatesInPreviewAndAfterReload:true,allHiddenBarEmpty:true,singleSelectionCentre:true,mobileNoOverflow:true,errors};fs.writeFileSync(q+'1.0.8-gpu-selection-browser.json',JSON.stringify(result,null,2));console.log(JSON.stringify(result));
 }finally{await browser.close();await new Promise(resolve=>server.close(resolve))}
})().catch(e=>{console.error(e);process.exitCode=1});
