// Products page: list, create and edit products.
(function () {
  const { api, escapeHTML, formatDate, toast, showFormError, clearFormError, busy, skeletonRows } = window.KS;

  const tableEl = document.getElementById('products-table');
  const bodyEl = document.getElementById('products-body');
  const emptyEl = document.getElementById('products-empty');
  const loadErrorEl = document.getElementById('load-error');

  const dialog = document.getElementById('product-dialog');
  const form = document.getElementById('product-form');
  const titleEl = document.getElementById('product-dialog-title');
  const submitEl = document.getElementById('product-submit');
  const nameEl = form.elements.name;
  const slugEl = form.elements.slug;
  const prefixEl = form.elements.keyPrefix;
  const descEl = form.elements.description;
  const slugHintEl = document.getElementById('p-slug-hint');

  let products = [];
  let editing = null; // the product being edited, or null when creating
  let slugTouched = false;

  function slugify(text) {
    return text.toLowerCase().normalize('NFKD').replace(/[̀-ͯ]/g, '')
      .replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 64);
  }

  function render() {
    emptyEl.hidden = products.length > 0;
    tableEl.hidden = products.length === 0;
    bodyEl.innerHTML = products.map((p) => `
      <tr>
        <td>
          <div class="font-medium">${escapeHTML(p.name)}</div>
          ${p.description ? `<div class="mt-0.5 max-w-xs truncate text-sm text-muted">${escapeHTML(p.description)}</div>` : ''}
        </td>
        <td class="mono text-muted">${escapeHTML(p.slug)}</td>
        <td class="mono">${escapeHTML(p.keyPrefix)}</td>
        <td>
          <a class="hover:underline" href="/licenses?product=${encodeURIComponent(p.id)}">
            ${p.activeLicenses.toLocaleString()} active<span class="text-muted"> · ${p.totalLicenses.toLocaleString()} total</span>
          </a>
        </td>
        <td class="whitespace-nowrap text-muted">${formatDate(p.createdAt)}</td>
        <td class="text-right"><button type="button" class="btn-secondary btn-sm" data-edit="${escapeHTML(p.id)}">Edit</button></td>
      </tr>`).join('');
  }

  async function load() {
    try {
      products = await api('/api/products');
      loadErrorEl.hidden = true;
      render();
    } catch (err) {
      if (err.status === 401) return;
      loadErrorEl.textContent = `Couldn't load products: ${err.message}`;
      loadErrorEl.hidden = false;
    }
  }

  function openDialog(product) {
    editing = product || null;
    form.reset();
    clearFormError(form);
    slugTouched = Boolean(product);
    titleEl.textContent = product ? 'Edit product' : 'New product';
    submitEl.textContent = product ? 'Save changes' : 'Create product';
    slugEl.disabled = Boolean(product);
    slugHintEl.textContent = product
      ? "The slug can't be changed, because plugins already send it with every license check."
      : "Your plugin sends this with every license check. It can't be changed later.";
    if (product) {
      nameEl.value = product.name;
      slugEl.value = product.slug;
      prefixEl.value = product.keyPrefix;
      descEl.value = product.description || '';
    }
    dialog.showModal();
    nameEl.focus();
  }

  nameEl.addEventListener('input', () => {
    if (!slugTouched) slugEl.value = slugify(nameEl.value);
  });
  slugEl.addEventListener('input', () => { slugTouched = true; });
  prefixEl.addEventListener('input', () => {
    prefixEl.value = prefixEl.value.toUpperCase().replace(/[^A-Z0-9]/g, '');
  });

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    clearFormError(form);
    const body = {
      name: nameEl.value,
      keyPrefix: prefixEl.value,
      description: descEl.value,
    };
    if (!editing) body.slug = slugEl.value;

    busy(submitEl, true, editing ? 'Saving…' : 'Creating…');
    try {
      if (editing) {
        await api(`/api/products?id=${encodeURIComponent(editing.id)}`, { method: 'PATCH', body });
        toast('Product saved.');
      } else {
        await api('/api/products', { method: 'POST', body });
        toast('Product created.');
      }
      dialog.close();
      load();
    } catch (err) {
      showFormError(form, err);
    } finally {
      busy(submitEl, false);
    }
  });

  dialog.addEventListener('click', (event) => {
    if (event.target.closest('[data-close]') || event.target === dialog) dialog.close();
  });

  document.addEventListener('click', (event) => {
    if (event.target.closest('#new-product, [data-action="new-product"]')) openDialog(null);
    const editId = event.target.closest('[data-edit]')?.dataset.edit;
    if (editId) openDialog(products.find((p) => p.id === editId));
  });

  // Show placeholder rows straight away, then load.
  tableEl.hidden = false;
  bodyEl.innerHTML = skeletonRows(6);
  load();
})();
