/* Progressive area selection using the existing geo and coverage endpoints. */
(() => {
 'use strict';
 const $=id=>document.getElementById(id);
 const p=$('province'),r=$('regency'),d=$('district'),v=$('village');
 const selects=[p,r,d,v], placeholders=['Pilih provinsi','Pilih kabupaten/kota','Pilih kecamatan','Pilih kelurahan'];
 const scrapeModes=[...document.querySelectorAll('input[name="scrape-mode"]')];
 const queryMode=$('scrape-query-mode'),manualPanel=$('manual-query-panel'),manualKeywords=$('manual-keywords'),manualIncludeDefaults=$('manual-include-defaults'),manualCount=$('manual-query-count');
 let geoController,coverageController,revision=0,mode='core',savedScope='',retry=()=>chooseCity('DKI JAKARTA','KOTA JAKARTA SELATAN');
 let scrapeTimer=null,scrapeRunning=false;
 const norm=s=>s.toUpperCase().replace(/DAERAH KHUSUS (IBUKOTA )?JAKARTA/g,'DKI JAKARTA').replace(/ADMINISTRASI|ADMINISTRATIF/g,'').replace(/[^A-Z0-9]+/g,' ').trim();
 const label=el=>el.value?el.selectedOptions[0].textContent.trim():'';
 const currentScope=()=>savedScope || [v,d,r,p].map(label).filter(Boolean).join(', ')+(p.value?', Indonesia':'');
 function link(id,href,enabled){const el=$(id);el.setAttribute('aria-disabled',String(!enabled));if(enabled){el.href=href;el.removeAttribute('tabindex');}else{el.removeAttribute('href');el.tabIndex=-1;}}
 function reset(index){for(let i=index;i<selects.length;i++){selects[i].replaceChildren(new Option(placeholders[i],''));selects[i].disabled=true;}}
 function customKeywords(){
  const seen=new Set(),out=[];
  (manualKeywords?.value||'').split(/[\n\r,;]+/).forEach(value=>{const keyword=value.trim();if(!keyword)return;const key=keyword.toLowerCase();if(seen.has(key))return;seen.add(key);out.push(keyword);});
  return out;
 }
 function manualMode(){return scrapeModes.some(input=>input.checked&&input.value==='manual');}
 function scrapeReady(){const keywords=customKeywords();return !!v.value&&(!manualMode()||(keywords.length>0&&keywords.length<=100&&keywords.every(x=>[...x].length<=120)));}
 function updateScrapeButton(){if($('scrape-btn'))$('scrape-btn').disabled=scrapeRunning||!scrapeReady();}
 function syncScrapeMode(){
  const manual=manualMode(),keywords=customKeywords();
  if(manualPanel)manualPanel.hidden=!manual;
  if(manualCount)manualCount.textContent=String(keywords.length);
  if(queryMode)queryMode.value=manual?(manualIncludeDefaults?.checked?'custom_plus_defaults':'custom'):'defaults_only';
  document.querySelectorAll('.scrape-mode-card').forEach(card=>card.classList.toggle('active',!!card.querySelector('input:checked')));
  updateScrapeButton();
 }
 function clearCoverage(message='Pilih sampai kelurahan untuk melihat coverage.'){
  coverageController?.abort();$('coverage-section').setAttribute('aria-busy','false');$('coverage-status').textContent='PILIH KELURAHAN';$('coverage-status').className='ui-badge slate';
  $('coverage-message').hidden=false;$('coverage-message').textContent=message;$('coverage-message').className='p2-message';
  ['total','unvisited','planned','visited','revisit','routable'].forEach(key=>$('st-'+key).textContent='—');
  $('progress-bar').value=0;$('progress-text').textContent='Coverage belum tersedia';$('progress-detail').textContent='';
  $('area-snapshot').replaceChildren();$('area-recommendations').replaceChildren();
  link('route-link','',false);link('merchant-link','',false);updateScrapeButton();
 }
 async function load(url,select,index,signal){
  select.replaceChildren(new Option('Memuat…',''));select.disabled=true;
  const res=await fetch(url,{signal});if(!res.ok)throw Error('Wilayah gagal dimuat. Silakan coba lagi.');
  const data=await res.json();if(signal.aborted)throw new DOMException('Aborted','AbortError');
  select.replaceChildren(new Option(placeholders[index],''),...data.map(x=>new Option(x.name,x.id)));select.disabled=false;
 }
 function selectName(select,name){const target=norm(name);const option=[...select.options].find(o=>o.value && norm(o.textContent)===target);if(!option)throw Error('Wilayah tersimpan tidak ditemukan di selector saat ini. Pilih wilayah secara manual.');select.value=option.value;}
 async function runGeo(task){
  geoController?.abort();geoController=new AbortController();const signal=geoController.signal;const token=++revision;
  savedScope='';clearCoverage('Memuat pilihan wilayah…');$('geo-error').hidden=true;$('scrape-message').hidden=true;
  retry=()=>runGeo(task);
  try {await task(signal);if(token!==revision)return;sync();return true;}
  catch(e){if(e.name==='AbortError'||token!==revision)return;$('scope').textContent='Pilihan wilayah belum lengkap';$('geo-error').hidden=false;$('geo-error').querySelector('span').textContent=e.message;clearCoverage('Wilayah belum berhasil dimuat. Gunakan tombol Coba lagi.');return false;}
 }
 function setMode(next){mode=next;['core','border','all'].forEach(x=>$(x+'-btn').setAttribute('aria-pressed',String(x===next)));$('border-options').hidden=next!=='border';$('saved-areas').hidden=next!=='all';}
 function chooseCity(province,city){return runGeo(async signal=>{reset(0);await load('/api/geo/provinces',p,0,signal);selectName(p,province);await load('/api/geo/regencies?province_id='+encodeURIComponent(p.value),r,1,signal);selectName(r,city);await load('/api/geo/districts?regency_id='+encodeURIComponent(r.value),d,2,signal);p.disabled=mode!=='all';r.disabled=mode!=='all';});}
 function sync(){const scope=currentScope();$('scope').textContent=scope||'Pilih area kerja';$('collect-location').value=scope;if(v.value||savedScope)loadCoverage(scope);else clearCoverage();syncScrapeMode();}
 async function restore(scope){
  if(!scope)return;setMode('all');
  const parts=scope.split(',').map(s=>s.trim());
  const restored=await runGeo(async signal=>{
   reset(0);await load('/api/geo/provinces',p,0,signal);
   if(parts.length<5)throw Error('Area tersimpan tidak memiliki empat tingkat wilayah. Pilih kelurahan secara manual.');
   selectName(p,parts[parts.length-2]);await load('/api/geo/regencies?province_id='+encodeURIComponent(p.value),r,1,signal);selectName(r,parts[parts.length-3]);
   await load('/api/geo/districts?regency_id='+encodeURIComponent(r.value),d,2,signal);selectName(d,parts[1]);
   await load('/api/geo/villages?district_id='+encodeURIComponent(d.value),v,3,signal);selectName(v,parts[0]);savedScope=scope;
  });
  // Historical area strings remain usable even if upstream names changed.
  // Scraping stays disabled until the complete village selector is available.
  if(restored===false){savedScope=scope;sync();}
 }
 p.addEventListener('change',()=>runGeo(async signal=>{reset(1);if(p.value)await load('/api/geo/regencies?province_id='+encodeURIComponent(p.value),r,1,signal);}));
 r.addEventListener('change',()=>runGeo(async signal=>{reset(2);if(r.value)await load('/api/geo/districts?regency_id='+encodeURIComponent(r.value),d,2,signal);}));
 d.addEventListener('change',()=>runGeo(async signal=>{reset(3);if(d.value)await load('/api/geo/villages?district_id='+encodeURIComponent(d.value),v,3,signal);}));
 v.addEventListener('change',()=>{savedScope='';clearCoverage();sync();});
 $('core-btn').onclick=()=>{setMode('core');chooseCity('DKI JAKARTA','KOTA JAKARTA SELATAN');};
 $('border-btn').onclick=()=>{setMode('border');chooseCity('DKI JAKARTA','KOTA JAKARTA PUSAT');};
 $('all-btn').onclick=()=>{setMode('all');runGeo(async signal=>{reset(0);await load('/api/geo/provinces',p,0,signal);});};
 document.querySelectorAll('[data-city]').forEach(b=>b.onclick=()=>chooseCity(b.dataset.province,b.dataset.city));
 $('saved-area').onchange=()=>restore($('saved-area').value);$('geo-retry').onclick=()=>retry();
 scrapeModes.forEach(input=>input.addEventListener('change',syncScrapeMode));
 manualKeywords?.addEventListener('input',syncScrapeMode);manualIncludeDefaults?.addEventListener('change',syncScrapeMode);
 function recommendation(text,href,label='Buka'){
  const box=document.createElement(href?'a':'div');box.className='area-next-action';if(href)box.href=href;
  const copy=document.createElement('span'),strong=document.createElement('strong'),small=document.createElement('small');strong.textContent=text;small.textContent=href?label:'Gunakan aksi utama pada halaman ini.';copy.append(strong,small);
  const icon=document.createElement('span');icon.className='ui-iconbox blue ui-round';const svg=document.createElementNS('http://www.w3.org/2000/svg','svg'),use=document.createElementNS('http://www.w3.org/2000/svg','use');svg.classList.add('ui-icon');use.setAttribute('href','#icon-arrow');svg.append(use);icon.append(svg);box.append(copy,icon);$('area-recommendations').replaceChildren(box);
 }
 function snapshot(count,text,icon='store',tone='blue'){
  const row=document.createElement('p'),box=document.createElement('span'),body=document.createElement('span'),b=document.createElement('strong');
  box.className='ui-iconbox '+tone;const svg=document.createElementNS('http://www.w3.org/2000/svg','svg'),use=document.createElementNS('http://www.w3.org/2000/svg','use');
  svg.classList.add('ui-icon');svg.setAttribute('aria-hidden','true');use.setAttribute('href','#icon-'+icon);svg.append(use);box.append(svg);
  b.textContent=count;body.append(b,' '+text);row.append(box,body);$('area-snapshot').append(row);
 }
 async function loadCoverage(scope){
  clearCoverage('Memuat coverage…');coverageController=new AbortController();const signal=coverageController.signal;
  $('coverage-section').setAttribute('aria-busy','true');$('coverage-status').textContent='MEMUAT';
  try{
   const res=await fetch('/api/coverage?location='+encodeURIComponent(scope),{signal,cache:'no-store'});if(!res.ok)throw Error('Coverage gagal dimuat. Pilih ulang kelurahan atau coba lagi.');
   const data=await res.json();if(signal.aborted)return;
   const q=data.progress,s=data.snapshot;
   const status=data.exists?q.Status:'not_scraped';
   $('coverage-status').textContent=({scraped:'TERSIMPAN',in_progress:'IN PROGRESS',completed:'COMPLETED',not_scraped:'BELUM DISCRAPE'})[status]||status;
   $('coverage-status').className='ui-badge '+(({completed:'green',in_progress:'amber'})[status]||'slate');
   $('coverage-message').hidden=data.exists;
   $('coverage-message').textContent='Kelurahan belum discrape. Mulai dengan Scrape Kelurahan Ini.';
   const keys={total:'Total',unvisited:'Unvisited',planned:'Planned',visited:'Visited',revisit:'RevisitRequired',routable:'Routable'};
   Object.entries(keys).forEach(([id,key])=>$('st-'+id).textContent=data.exists?q[key]:'—');
   $('progress-bar').value=q.ProgressPercent;
   $('progress-text').textContent=data.exists?q.ProgressPercent.toFixed(1)+'% coverage selesai':'Coverage belum tersedia';
   $('progress-detail').textContent=data.exists?`${q.Visited} visited + ${q.Excluded} excluded dari ${q.Total} merchant`:'';
   const encoded=encodeURIComponent(scope),route='/visit-plans/new?location='+encoded;
   updateScrapeButton();link('merchant-link','/merchants?location='+encoded,true);
   link('route-link',s.TomorrowPlanID?'/visit-plan/'+s.TomorrowPlanID:route,s.TomorrowPlanID>0||s.EligibleTomorrow>0);
   $('route-link').querySelector('span').textContent=s.TomorrowPlanID?'Lihat Rute Besok':'Buat Rute Besok';
   $('area-snapshot').replaceChildren();snapshot(s.WithPhone,'merchant dengan telepon','phone','green');snapshot(s.WithMaps,'merchant dengan tautan Maps','map');
   (s.Categories||[]).forEach(c=>snapshot(c.Count,'merchant · '+c.Name));snapshot(s.MissingCoordinates,'merchant tanpa koordinat valid','pin','amber');snapshot(s.EligibleTomorrow,'merchant siap dirutekan besok','route','purple');

   const next=[...$('saved-area').options].find(o=>o.value&&o.value!==scope&&Number(o.dataset.remaining)>0);
   if(!data.exists)recommendation('Scrape kelurahan ini untuk membangun database merchant.',null,'Scrape Kelurahan Ini');
   else if(status==='completed'&&next)recommendation('Coverage selesai. Lanjut ke area berikutnya: '+next.value,'/areas?location='+encodeURIComponent(next.value),'Pilih Area Berikutnya');
   else if(status==='completed')recommendation('Coverage area ini selesai. Pilih area kerja berikutnya.','/areas','Pilih Area Berikutnya');
   else if(s.TomorrowPlanID)recommendation('Rute besok sudah tersimpan.','/visit-plan/'+s.TomorrowPlanID,'Lihat Rute Besok');
   else if(s.EligibleTomorrow>0)recommendation(`Siapkan rute besok dari ${s.EligibleTomorrow} merchant yang siap dikunjungi.`,route,'Buat Rute Besok');
   else if(q.RevisitRequired>0)recommendation(`${q.RevisitRequired} merchant perlu revisit sesuai jadwal.`,'/contact?mode=follow_up','Buka Revisit Queue');
   else if(s.MissingCoordinates>0)recommendation(`${s.MissingCoordinates} merchant belum memiliki koordinat valid.`,'/database?location='+encoded,'Periksa Database');
   else recommendation('Belum ada kandidat rute baru. Tinjau merchant area ini.','/merchants?location='+encoded,'Lihat Merchant Area');
  }catch(e){if(e.name==='AbortError')return;clearCoverage(e.message);$('coverage-status').textContent='ERROR';$('coverage-message').className='p2-message p2-error';const b=document.createElement('button');b.type='button';b.className='ui-button ui-compact';b.textContent='Coba lagi';b.onclick=()=>loadCoverage(scope);$('coverage-message').append(' ',b);}
  finally{if(!signal.aborted)$('coverage-section').setAttribute('aria-busy','false');syncScrapeMode();}
 }
 function elapsedLabel(seconds){seconds=Number(seconds)||0;const m=Math.floor(seconds/60),s=seconds%60;return m?`${m}m ${s}s`:`${s}s`;}
 function renderScrapeState(state){
  const box=$('scrape-runtime'),phase=$('scrape-phase'),text=$('scrape-status-text'),progress=$('scrape-progress'),elapsed=$('scrape-elapsed'),terminal=$('scrape-terminal'),cancel=$('scrape-cancel'),logWrap=$('scrape-log-wrap'),log=$('scrape-log');
  const hasState=!!(state&&(state.running||state.started_unix||state.phase||state.message));box.hidden=!hasState;if(!hasState)return;
  phase.textContent=state.phase||'Menyiapkan';text.textContent=state.message||'Collector siap.';elapsed.textContent=state.started_unix?'Durasi '+elapsedLabel(state.elapsed_seconds):'';
  if(state.running){progress.removeAttribute('value');terminal.hidden=true;}else{progress.value=(state.phase==='Selesai')?100:0;terminal.hidden=false;terminal.textContent=(state.phase||'READY').toUpperCase();terminal.className='ui-badge '+(state.phase==='Selesai'?'green':state.phase==='Gagal'?'rose':state.phase==='Dibatalkan'?'amber':'slate');}
  cancel.hidden=!state.can_cancel;cancel.disabled=!!state.cancel_requested;
  log.textContent=state.log_tail||'';logWrap.hidden=!state.log_tail;
 }
 async function pollScrapeStatus(){
  clearTimeout(scrapeTimer);
  try{
   const res=await fetch('/api/collect/status',{cache:'no-store'});if(!res.ok)throw Error('Status scraper gagal dimuat.');
   const state=await res.json(),wasRunning=scrapeRunning;scrapeRunning=!!state.running;renderScrapeState(state);updateScrapeButton();
   if(state.running){scrapeTimer=setTimeout(pollScrapeStatus,1500);return;}
   if(wasRunning&&state.started_unix){const scope=currentScope();if(scope)await loadCoverage(scope);}
  }catch(err){$('scrape-runtime').hidden=false;$('scrape-status-text').textContent=err.message;scrapeRunning=false;updateScrapeButton();}
 }
 $('scrape-cancel').onclick=async()=>{
  $('scrape-cancel').disabled=true;
  try{const res=await fetch('/collect/cancel',{method:'POST'});if(!res.ok)throw Error(await res.text());const state=await res.json();scrapeRunning=!!state.running;renderScrapeState(state);scrapeTimer=setTimeout(pollScrapeStatus,1000);}catch(err){$('scrape-status-text').textContent=err.message;$('scrape-cancel').disabled=false;}
 };
 $('scrape-form').addEventListener('submit',async e=>{
  e.preventDefault();syncScrapeMode();
  const keywords=customKeywords();
  if(manualMode()&&keywords.length===0){$('scrape-message').hidden=false;$('scrape-message').className='p2-message p2-error';$('scrape-message').textContent='Isi minimal satu custom query untuk mode Manual.';manualKeywords?.focus();return;}
  if(keywords.length>100){$('scrape-message').hidden=false;$('scrape-message').className='p2-message p2-error';$('scrape-message').textContent='Maksimal 100 custom query per sekali scrape.';manualKeywords?.focus();return;}
  if(keywords.some(x=>[...x].length>120)){$('scrape-message').hidden=false;$('scrape-message').className='p2-message p2-error';$('scrape-message').textContent='Setiap custom query maksimal 120 karakter.';manualKeywords?.focus();return;}
  if($('scrape-btn').disabled||!v.value)return;
  $('scrape-message').hidden=true;scrapeRunning=true;updateScrapeButton();$('scrape-runtime').hidden=false;$('scrape-phase').textContent='Menyiapkan';$('scrape-status-text').textContent=manualMode()?`Menjalankan ${keywords.length} custom query${manualIncludeDefaults?.checked?' + preset Bukupay':''}…`:'Menjalankan preset otomatis Bukupay…';$('scrape-progress').removeAttribute('value');
  try{const res=await fetch('/bukupay/collect',{method:'POST',body:new URLSearchParams(new FormData(e.target))});if(!res.ok)throw Error((await res.text()).trim()||'Scrape belum dapat dimulai.');const data=await res.json();if(!data.started)throw Error('Scrape belum dapat dimulai.');renderScrapeState(data.state||{});scrapeTimer=setTimeout(pollScrapeStatus,1000);}
  catch(err){scrapeRunning=false;$('scrape-runtime').hidden=false;$('scrape-phase').textContent='Gagal';$('scrape-status-text').textContent=err.message;$('scrape-progress').value=0;updateScrapeButton();}
 });
 syncScrapeMode();pollScrapeStatus();
 const initial=$('main').dataset.initialScope;if(initial)restore(initial);else chooseCity('DKI JAKARTA','KOTA JAKARTA SELATAN');
})();