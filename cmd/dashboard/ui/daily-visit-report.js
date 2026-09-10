(() => {
  if (window.location.pathname !== '/contact') return;
  const template = document.getElementById('daily-visit-report-template');
  const main = document.getElementById('main');
  if (!template || !main) return;

  const fragment = template.content.cloneNode(true);
  main.append(fragment);

  const card = document.getElementById('daily-visit-report');
  const form = document.getElementById('daily-report-form');
  const dateInput = document.getElementById('daily-report-date');
  const areaSelect = document.getElementById('daily-report-area');
  const status = document.getElementById('daily-report-status');
  const output = document.getElementById('daily-report-output');
  const text = document.getElementById('daily-report-text');
  const copyButton = document.getElementById('daily-report-copy');
  const copyStatus = document.getElementById('daily-report-copy-status');
  if (!card || !form || !dateInput || !areaSelect || !status || !output || !text || !copyButton) return;

  const todayLocal = () => {
    const now = new Date();
    const y = now.getFullYear();
    const m = String(now.getMonth() + 1).padStart(2, '0');
    const d = String(now.getDate()).padStart(2, '0');
    return `${y}-${m}-${d}`;
  };
  dateInput.value = todayLocal();

  let areasLoaded = false;
  let loadedOnce = false;

  function populateAreas(areas, selected) {
    if (!Array.isArray(areas)) return;
    const current = selected || areaSelect.value;
    areaSelect.replaceChildren();
    const all = document.createElement('option');
    all.value = '';
    all.textContent = 'Semua Area';
    areaSelect.appendChild(all);
    for (const area of areas) {
      const option = document.createElement('option');
      option.value = area;
      option.textContent = area.split(',').slice(0, 2).join(',').trim();
      if (area === current) option.selected = true;
      areaSelect.appendChild(option);
    }
    areasLoaded = true;
  }

  async function loadReport() {
    status.textContent = 'Menyiapkan laporan…';
    output.hidden = true;
    copyStatus.textContent = '';
    const params = new URLSearchParams();
    if (dateInput.value) params.set('date', dateInput.value);
    if (areaSelect.value) params.set('location', areaSelect.value);
    try {
      const response = await fetch(`/api/visit-report?${params.toString()}`, { headers: { Accept: 'application/json' } });
      if (!response.ok) throw new Error(await response.text());
      const data = await response.json();
      if (!areasLoaded) populateAreas(data.areas || [], data.location || '');
      text.value = data.text || 'Belum ada laporan.';
      status.textContent = `${data.total_visited || 0} merchant dikunjungi · ${data.route_done || 0}/${data.route_target || 0} rute selesai`;
      output.hidden = false;
      loadedOnce = true;
    } catch (error) {
      status.textContent = `Laporan gagal dimuat: ${String(error.message || error).trim()}`;
    }
  }

  card.addEventListener('toggle', () => {
    if (card.open && !loadedOnce) loadReport();
  });

  form.addEventListener('submit', (event) => {
    event.preventDefault();
    loadReport();
  });

  copyButton.addEventListener('click', async () => {
    const value = text.value;
    if (!value) return;
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(value);
      } else {
        text.focus();
        text.select();
        document.execCommand('copy');
        text.setSelectionRange(0, 0);
      }
      copyStatus.textContent = 'Laporan berhasil disalin.';
    } catch (error) {
      copyStatus.textContent = 'Gagal menyalin. Pilih teks lalu salin manual.';
    }
  });
})();
