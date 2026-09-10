/* Area Irisan guardrail: expose only districts/villages that stay close to Jaksel core. */
(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  const p = $('province'), r = $('regency'), d = $('district'), v = $('village');
  if (!p || !r || !d || !v) return;

  const norm = value => String(value || '')
    .toUpperCase()
    .replace(/DAERAH KHUSUS (IBUKOTA )?JAKARTA/g, 'DKI JAKARTA')
    .replace(/ADMINISTRASI|ADMINISTRATIF/g, '')
    .replace(/[^A-Z0-9]+/g, ' ')
    .trim();

  // Operational whitelist, intentionally narrower than the full surrounding city.
  // It keeps canvassing close to the Jakarta Selatan core instead of exposing
  // every district/village in Depok, Tangsel, Jakarta Barat/Timur/Pusat.
  const borderAreas = {
    'KOTA JAKARTA PUSAT': {
      'TANAH ABANG': ['BENDUNGAN HILIR', 'KARET TENGSIN', 'GELORA'],
      'MENTENG': ['MENTENG', 'PEGANGSAAN'],
    },
    'KOTA JAKARTA BARAT': {
      'PALMERAH': ['PALMERAH', 'KEMANGGISAN'],
      'KEBON JERUK': ['SUKABUMI SELATAN', 'KELAPA DUA'],
      'KEMBANGAN': ['JOGLO', 'MERUYA SELATAN', 'SRENGSENG'],
    },
    'KOTA JAKARTA TIMUR': {
      'JATINEGARA': ['BIDARA CINA', 'KAMPUNG MELAYU'],
      'KRAMAT JATI': ['CAWANG', 'CILILITAN'],
      'PASAR REBO': ['CIJANTUNG', 'GEDONG', 'KALISARI'],
    },
    'KOTA DEPOK': {
      'BEJI': ['KUKUSAN', 'TANAH BARU', 'PONDOK CINA'],
      'CINERE': ['CINERE', 'GANDUL', 'PANGKALAN JATI', 'PANGKALAN JATI BARU'],
      'LIMO': ['LIMO', 'GROGOL', 'KRUKUT'],
    },
    'KOTA TANGERANG SELATAN': {
      'CIPUTAT TIMUR': ['CIREUNDEU', 'PONDOK RANJI', 'RENGAS', 'REMPOA'],
      'PONDOK AREN': ['PONDOK BETUNG', 'PONDOK KARYA', 'PONDOK PUCUNG'],
      'CIPUTAT': ['BINTARO', 'SAWAH', 'SAWAH BARU'],
    },
  };

  const selectedName = select => norm(select.selectedOptions?.[0]?.textContent || '');
  const borderMode = () => $('border-btn')?.getAttribute('aria-pressed') === 'true';

  function showConfigError(message) {
    const box = $('geo-error');
    if (!box) return;
    box.hidden = false;
    const span = box.querySelector('span');
    if (span) span.textContent = message;
  }

  function restrictOptions(select, allowed, fallbackLabel) {
    if (!Array.isArray(allowed) || allowed.length === 0) return;
    const allowedSet = new Set(allowed.map(norm));
    const existing = [...select.options].filter(option => option.value);
    if (existing.length === 0) return;

    const matches = existing.filter(option => allowedSet.has(norm(option.textContent)));
    if (matches.length === 0) {
      const placeholder = select.options[0]?.cloneNode(true) || new Option(fallbackLabel, '');
      select.replaceChildren(placeholder);
      select.disabled = true;
      showConfigError('Whitelist Area Irisan tidak cocok dengan data wilayah terbaru. Gunakan Semua Area sementara dan periksa konfigurasi.');
      return;
    }

    const currentNames = existing.map(option => norm(option.textContent));
    const matchNames = matches.map(option => norm(option.textContent));
    if (currentNames.length === matchNames.length && currentNames.every((name, i) => name === matchNames[i])) return;

    const selected = select.value;
    const placeholder = select.options[0]?.cloneNode(true) || new Option(fallbackLabel, '');
    const clones = matches.map(option => option.cloneNode(true));
    select.replaceChildren(placeholder, ...clones);
    if (clones.some(option => option.value === selected)) select.value = selected;
    select.disabled = false;
  }

  function filterDistricts() {
    if (!borderMode()) return;
    const city = selectedName(r);
    const config = borderAreas[city];
    if (!config) return;
    restrictOptions(d, Object.keys(config), 'Pilih kecamatan dekat Jaksel');
  }

  function filterVillages() {
    if (!borderMode()) return;
    const city = selectedName(r);
    const district = selectedName(d);
    const allowed = borderAreas[city]?.[district];
    if (!allowed) return;
    restrictOptions(v, allowed, 'Pilih kelurahan dekat Jaksel');
  }

  let filteringDistricts = false;
  let filteringVillages = false;
  new MutationObserver(() => {
    if (filteringDistricts) return;
    filteringDistricts = true;
    queueMicrotask(() => {
      filterDistricts();
      filteringDistricts = false;
    });
  }).observe(d, { childList: true });

  new MutationObserver(() => {
    if (filteringVillages) return;
    filteringVillages = true;
    queueMicrotask(() => {
      filterVillages();
      filteringVillages = false;
    });
  }).observe(v, { childList: true });

  d.addEventListener('change', () => queueMicrotask(filterVillages));
  document.querySelectorAll('#border-btn,[data-city]').forEach(button => {
    button.addEventListener('click', () => {
      setTimeout(filterDistricts, 0);
      setTimeout(filterVillages, 0);
    });
  });
})();
