// Licenses page: search, issue, edit, revoke/restore, and manage machines.
(function () {
  const {
    api, escapeHTML, formatDate, formatDateTime, endOfDayISO, toDateInput,
    toast, copy, showFormError, clearFormError, busy, statePill, skeletonRows,
  } = window.KS;

  const PAGE_SIZE = 50;
  const $ = (id) => document.getElementById(id);

  // ---------------------------------------------------------------- list
  const filtersForm = $('filters');
  const qEl = $('f-q');
  const productFilterEl = $('f-product');
  const stateFilterEl = $('f-state');
  const tableEl = $('licenses-table');
  const bodyEl = $('licenses-body');
  const footerEl = $('list-footer');
  const countEl = $('list-count');
  const moreEl = $('load-more');
  const emptyEl = $('licenses-empty');
  const loadErrorEl = $('load-error');

  let products = [];
  let pendingProduct = ''; // product filter from the URL, until the list loads
  let items = [];
  let total = 0;
  let requestSeq = 0;

  function filters() {
    return { q: qEl.value.trim(), product: productFilterEl.value || pendingProduct, state: stateFilterEl.value };
  }

  // Keep filters in the address bar, so a refresh or shared link keeps them.
  function syncURL() {
    const params = new URLSearchParams();
    for (const [k, v] of Object.entries(filters())) if (v) params.set(k, v);
    const qs = params.toString();
    history.replaceState(null, '', qs ? `?${qs}` : location.pathname);
  }

  function rowHTML(l) {
    const customer = l.customerName || l.customerEmail
      ? `<div>${escapeHTML(l.customerName || l.customerEmail)}</div>${l.customerName && l.customerEmail ? `<div class="text-sm text-muted">${escapeHTML(l.customerEmail)}</div>` : ''}`
      : '<span class="text-faint">—</span>';
    return `
      <tr class="clickable" data-id="${escapeHTML(l.id)}" tabindex="0" aria-label="Open license ${escapeHTML(l.key)}">
        <td class="mono whitespace-nowrap">${escapeHTML(l.key)}</td>
        <td class="whitespace-nowrap">${escapeHTML(l.product.name)}</td>
        <td>${customer}</td>
        <td class="whitespace-nowrap tabular-nums">${l.activations} <span class="text-muted">/ ${l.maxActivations}</span></td>
        <td class="whitespace-nowrap ${l.state === 'expired' ? '' : 'text-muted'}">${l.expiresAt ? formatDate(l.expiresAt) : 'Never'}</td>
        <td>${statePill(l.state)}</td>
      </tr>`;
  }

  function render() {
    const f = filters();
    const filtering = Boolean(f.q || f.product || f.state);
    tableEl.hidden = items.length === 0;
    footerEl.hidden = items.length === 0;
    emptyEl.hidden = items.length > 0;
    bodyEl.innerHTML = items.map(rowHTML).join('');
    countEl.textContent = `Showing ${items.length.toLocaleString()} of ${total.toLocaleString()}`;
    moreEl.hidden = items.length >= total;

    if (items.length === 0) {
      $('empty-title').textContent = filtering ? 'No matching licenses' : 'No licenses yet';
      $('empty-text').textContent = filtering
        ? 'Nothing matches these filters. Try a shorter search or a different status.'
        : 'Issue a license to give a customer a key for one of your products.';
      $('empty-action').textContent = filtering ? 'Clear filters' : 'Issue license';
      $('empty-action').dataset.mode = filtering ? 'clear' : 'issue';
    }
  }

  async function loadLicenses({ append = false } = {}) {
    const seq = ++requestSeq;
    const f = filters();
    const params = new URLSearchParams({ limit: PAGE_SIZE, offset: append ? items.length : 0 });
    for (const [k, v] of Object.entries(f)) if (v) params.set(k, v);
    try {
      const res = await api(`/api/licenses?${params}`);
      if (seq !== requestSeq) return; // a newer search has started
      items = append ? items.concat(res.items) : res.items;
      total = res.total;
      loadErrorEl.hidden = true;
      render();
    } catch (err) {
      if (err.status === 401 || seq !== requestSeq) return;
      loadErrorEl.textContent = `Couldn't load licenses: ${err.message}`;
      loadErrorEl.hidden = false;
    }
  }

  async function loadProducts() {
    products = await api('/api/products');
    const options = products.map((p) => `<option value="${escapeHTML(p.id)}">${escapeHTML(p.name)}</option>`).join('');
    const selected = productFilterEl.value;
    productFilterEl.innerHTML = `<option value="">All products</option>${options}`;
    productFilterEl.value = selected;
    $('i-product').innerHTML = options;
  }

  let searchTimer;
  qEl.addEventListener('input', () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => { syncURL(); loadLicenses(); }, 250);
  });
  filtersForm.addEventListener('change', (event) => {
    if (event.target === qEl) return;
    syncURL();
    loadLicenses();
  });
  moreEl.addEventListener('click', async () => {
    busy(moreEl, true, 'Loading…');
    await loadLicenses({ append: true });
    busy(moreEl, false);
  });
  $('empty-action').addEventListener('click', (event) => {
    if (event.currentTarget.dataset.mode === 'clear') {
      filtersForm.reset();
      syncURL();
      loadLicenses();
    } else {
      openIssue();
    }
  });

  bodyEl.addEventListener('click', (event) => {
    const row = event.target.closest('tr[data-id]');
    if (row) openDetail(row.dataset.id);
  });
  bodyEl.addEventListener('keydown', (event) => {
    const row = event.target.closest('tr[data-id]');
    if (row && (event.key === 'Enter' || event.key === ' ')) {
      event.preventDefault();
      openDetail(row.dataset.id);
    }
  });

  // ---------------------------------------------------------------- issue
  const issueDialog = $('issue-dialog');
  const issueForm = $('issue-form');
  const issueSubmit = $('issue-submit');
  let issuedKey = '';

  function showIssueStep(done) {
    $('issue-fields').hidden = done;
    $('issue-foot').hidden = done;
    $('issue-done').hidden = !done;
    $('issue-done-foot').hidden = !done;
    $('issue-title').textContent = done ? 'License issued' : 'Issue license';
  }

  function openIssue() {
    const keepProduct = issueForm.elements.productId.value || productFilterEl.value;
    issueForm.reset();
    clearFormError(issueForm);
    showIssueStep(false);
    if (keepProduct) issueForm.elements.productId.value = keepProduct;
    const none = products.length === 0;
    $('issue-no-products').hidden = !none;
    issueSubmit.disabled = none;
    issueDialog.showModal();
    (none ? issueDialog.querySelector('a') : issueForm.elements.productId).focus();
  }

  issueForm.addEventListener('submit', async (event) => {
    event.preventDefault();
    clearFormError(issueForm);
    const el = issueForm.elements;
    const body = {
      productId: el.productId.value,
      customerName: el.customerName.value,
      customerEmail: el.customerEmail.value,
      note: el.note.value,
      maxActivations: Number(el.maxActivations.value) || 0,
      expiresAt: endOfDayISO(el.expiresAt.value),
    };
    busy(issueSubmit, true, 'Issuing…');
    try {
      const lic = await api('/api/licenses', { method: 'POST', body });
      issuedKey = lic.key;
      $('issued-key').textContent = lic.key;
      const parts = [lic.product.name, `${lic.maxActivations} machine${lic.maxActivations === 1 ? '' : 's'}`,
        lic.expiresAt ? `expires ${formatDate(lic.expiresAt)}` : 'never expires'];
      if (lic.customerEmail || lic.customerName) parts.push(`for ${lic.customerName || lic.customerEmail}`);
      $('issued-summary').textContent = parts.join(' · ');
      showIssueStep(true);
      loadLicenses();
    } catch (err) {
      showFormError(issueForm, err);
    } finally {
      busy(issueSubmit, false);
    }
  });

  $('copy-issued').addEventListener('click', () => copy(issuedKey, 'License key copied'));
  $('issue-another').addEventListener('click', openIssue);
  $('issue-license').addEventListener('click', openIssue);

  // ---------------------------------------------------------------- detail
  const detailDialog = $('detail-dialog');
  const editForm = $('edit-form');
  const saveEl = $('save-license');
  const revokeEl = $('toggle-revoke');
  let current = null;

  function renderDetail(lic) {
    current = lic;
    $('detail-title').textContent = lic.customerName || lic.customerEmail || 'License';
    $('detail-product').textContent = lic.product.name;
    $('detail-key').textContent = lic.key;
    $('detail-state').innerHTML = statePill(lic.state);
    $('detail-created').textContent = `Issued ${formatDateTime(lic.createdAt)}`;
    $('detail-revoked').textContent = lic.revokedAt ? `Revoked ${formatDateTime(lic.revokedAt)}` : '';

    const used = lic.activations.length;
    $('machines-count').textContent = `${used} of ${lic.maxActivations} in use`;
    $('machines-empty').hidden = used > 0;
    $('machines-list').hidden = used === 0;
    $('machines-list').innerHTML = lic.activations.map((a) => `
      <li class="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
        <div class="min-w-0">
          <div class="font-medium">${escapeHTML(a.label || 'Unnamed machine')}</div>
          <div class="mono truncate text-sm text-muted" title="${escapeHTML(a.fingerprint)}">${escapeHTML(a.fingerprint)}</div>
          <div class="mt-0.5 text-sm text-muted">First seen ${formatDateTime(a.createdAt)} · last check ${formatDateTime(a.lastSeenAt)}</div>
        </div>
        <button type="button" class="btn-danger btn-sm" data-remove="${escapeHTML(a.id)}">Remove</button>
      </li>`).join('');

    const el = editForm.elements;
    el.customerName.value = lic.customerName || '';
    el.customerEmail.value = lic.customerEmail || '';
    el.maxActivations.value = lic.maxActivations;
    el.expiresAt.value = toDateInput(lic.expiresAt);
    el.note.value = lic.note || '';
    clearFormError(editForm);

    const revoked = lic.status === 'revoked';
    revokeEl.textContent = revoked ? 'Restore license' : 'Revoke license';
    revokeEl.className = revoked ? 'btn-secondary' : 'btn-danger';
  }

  async function openDetail(id) {
    try {
      renderDetail(await api(`/api/licenses?id=${encodeURIComponent(id)}`));
      if (!detailDialog.open) detailDialog.showModal();
    } catch (err) {
      if (err.status !== 401) toast(`Couldn't open the license: ${err.message}`, 'error');
    }
  }

  async function patchLicense(body, button, busyText, doneText) {
    clearFormError(editForm);
    busy(button, true, busyText);
    try {
      renderDetail(await api(`/api/licenses?id=${encodeURIComponent(current.id)}`, { method: 'PATCH', body }));
      toast(doneText);
      loadLicenses();
    } catch (err) {
      showFormError(editForm, err);
    } finally {
      busy(button, false);
      // busy() restores the old label; the license may have changed state.
      if (current) revokeEl.textContent = current.status === 'revoked' ? 'Restore license' : 'Revoke license';
    }
  }

  editForm.addEventListener('submit', (event) => {
    event.preventDefault();
    const el = editForm.elements;
    patchLicense({
      customerName: el.customerName.value,
      customerEmail: el.customerEmail.value,
      maxActivations: Number(el.maxActivations.value) || 0,
      expiresAt: endOfDayISO(el.expiresAt.value),
      note: el.note.value,
    }, saveEl, 'Saving…', 'License saved.');
  });

  revokeEl.addEventListener('click', () => {
    if (current.status === 'revoked') {
      patchLicense({ status: 'active' }, revokeEl, 'Restoring…', 'License restored.');
    } else if (confirm(`Revoke ${current.key}? The customer's plugin will stop working the next time it checks the key. You can restore it later.`)) {
      patchLicense({ status: 'revoked' }, revokeEl, 'Revoking…', 'License revoked.');
    }
  });

  $('machines-list').addEventListener('click', async (event) => {
    const button = event.target.closest('[data-remove]');
    if (!button) return;
    if (!confirm('Remove this machine? It frees up a slot. If the machine checks the key again, it will take a slot again.')) return;
    busy(button, true, 'Removing…');
    try {
      await api(`/api/activations?id=${encodeURIComponent(button.dataset.remove)}`, { method: 'DELETE' });
      toast('Machine removed.');
      await openDetail(current.id);
      loadLicenses();
    } catch (err) {
      busy(button, false);
      if (err.status !== 401) toast(`Couldn't remove the machine: ${err.message}`, 'error');
    }
  });

  $('copy-detail').addEventListener('click', () => copy(current.key, 'License key copied'));

  // ---------------------------------------------------------------- dialogs
  for (const dialog of [issueDialog, detailDialog]) {
    dialog.addEventListener('click', (event) => {
      if (event.target.closest('[data-close]') || event.target === dialog) dialog.close();
    });
  }

  // ---------------------------------------------------------------- start
  const initial = new URLSearchParams(location.search);
  qEl.value = initial.get('q') || '';
  stateFilterEl.value = initial.get('state') || '';

  pendingProduct = initial.get('product') || '';

  // Show placeholder rows, then load products and licenses at the same time.
  tableEl.hidden = false;
  bodyEl.innerHTML = skeletonRows(6);
  loadLicenses();
  loadProducts()
    .then(() => {
      productFilterEl.value = pendingProduct;
      pendingProduct = '';
    })
    .catch((err) => {
      if (err.status === 401) return;
      loadErrorEl.textContent = `Couldn't load products: ${err.message}`;
      loadErrorEl.hidden = false;
    });
})();
