/**
 * transaction_log.js -- Chronicle Transaction Log Widget
 *
 * Displays a paginated list of shop transactions (purchases, sales, gifts,
 * restocks) for a given entity. Lazy-loads from the transactions API.
 *
 * Mount via:
 *   <div data-widget="transaction_log"
 *        data-transactions-endpoint="/campaigns/:id/armory/shops/:eid/transactions"
 *        data-campaign-url="/campaigns/:id"
 *        data-entity-id="..."
 *   ></div>
 */
Chronicle.register('transaction_log', {
  init: function (el) {
    var txEndpoint = el.dataset.transactionsEndpoint;
    var campaignUrl = el.dataset.campaignUrl;

    var state = {
      transactions: [],
      loading: true,
      total: 0,
      page: 1,
    };
    el._txState = state;

    // --- Styles ---

    var style = document.createElement('style');
    style.textContent = [
      '.tx-log { font-size: 0.875rem; }',
      '.tx-log-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem; }',
      '.tx-log-header h3 { font-size: 0.875rem; font-weight: 600; margin: 0; color: var(--text-primary, #1f2937); }',
      '.dark .tx-log-header h3 { color: #e5e7eb; }',
      '.tx-log-empty { color: #9ca3af; font-style: italic; padding: 1rem; text-align: center; }',
      '.tx-log-list { display: flex; flex-direction: column; gap: 0.375rem; }',
      '.tx-log-row { display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem; border-radius: 0.375rem; background: var(--bg-secondary, #f9fafb); border: 1px solid var(--border, #e5e7eb); }',
      '.dark .tx-log-row { background: #1f2937; border-color: #374151; }',
      '.tx-log-icon { width: 1.75rem; height: 1.75rem; display: flex; align-items: center; justify-content: center; border-radius: 0.25rem; flex-shrink: 0; font-size: 0.75rem; }',
      '.tx-log-icon.purchase { background: #d1fae520; color: #059669; }',
      '.tx-log-icon.sale { background: #dbeafe20; color: #2563eb; }',
      '.tx-log-icon.gift { background: #ede9fe20; color: #7c3aed; }',
      '.tx-log-icon.restock { background: #fef3c720; color: #d97706; }',
      '.tx-log-icon.transfer { background: #f3f4f620; color: #6b7280; }',
      '.tx-log-info { flex: 1; min-width: 0; }',
      '.tx-log-title { font-weight: 500; color: var(--text-primary, #1f2937); font-size: 0.8125rem; }',
      '.dark .tx-log-title { color: #e5e7eb; }',
      '.tx-log-meta { font-size: 0.6875rem; color: #6b7280; margin-top: 1px; }',
      '.tx-log-price { font-weight: 600; color: #d97706; white-space: nowrap; font-size: 0.8125rem; }',
      '.tx-log-date { font-size: 0.6875rem; color: #9ca3af; white-space: nowrap; }',
      '.tx-log-more { text-align: center; margin-top: 0.5rem; }',
      '.tx-log-more-btn { font-size: 0.75rem; padding: 0.25rem 0.75rem; border-radius: 0.25rem; background: #f3f4f6; color: #6b7280; border: 1px solid #e5e7eb; cursor: pointer; }',
      '.dark .tx-log-more-btn { background: #374151; color: #9ca3af; border-color: #4b5563; }',
      '.tx-log-more-btn:hover { background: #e5e7eb; }',
    ].join('\n');
    el.appendChild(style);

    // --- Render ---

    function render() {
      Array.from(el.children).forEach(function (child) {
        if (child.tagName !== 'STYLE') el.removeChild(child);
      });

      var wrap = document.createElement('div');
      wrap.className = 'tx-log';

      // Header.
      var header = document.createElement('div');
      header.className = 'tx-log-header';
      var h3 = document.createElement('h3');
      h3.innerHTML = '<i class="fa-solid fa-receipt" style="margin-right:0.375rem;opacity:0.6;font-size:0.75rem"></i>Transactions';
      header.appendChild(h3);
      wrap.appendChild(header);

      // Content.
      if (state.loading) {
        var loading = document.createElement('div');
        loading.className = 'tx-log-empty';
        loading.textContent = 'Loading transactions...';
        wrap.appendChild(loading);
      } else if (state.transactions.length === 0) {
        var empty = document.createElement('div');
        empty.className = 'tx-log-empty';
        empty.textContent = 'No transactions recorded.';
        wrap.appendChild(empty);
      } else {
        var list = document.createElement('div');
        list.className = 'tx-log-list';
        for (var i = 0; i < state.transactions.length; i++) {
          list.appendChild(renderRow(state.transactions[i]));
        }
        wrap.appendChild(list);

        // Load more button.
        if (state.transactions.length < state.total) {
          var more = document.createElement('div');
          more.className = 'tx-log-more';
          var moreBtn = document.createElement('button');
          moreBtn.className = 'tx-log-more-btn';
          moreBtn.textContent = 'Load more';
          moreBtn.addEventListener('click', function () {
            state.page++;
            loadTransactions(true);
          });
          more.appendChild(moreBtn);
          wrap.appendChild(more);
        }
      }

      el.appendChild(wrap);
    }

    function renderRow(tx) {
      var row = document.createElement('div');
      row.className = 'tx-log-row';

      // Type icon.
      var icon = document.createElement('div');
      icon.className = 'tx-log-icon ' + (tx.transaction_type || 'purchase');
      var iconMap = {
        purchase: 'fa-cart-shopping',
        sale: 'fa-coins',
        gift: 'fa-gift',
        restock: 'fa-boxes-stacked',
        transfer: 'fa-right-left',
      };
      icon.innerHTML = '<i class="fa-solid ' + (iconMap[tx.transaction_type] || 'fa-receipt') + '"></i>';
      row.appendChild(icon);

      // Info.
      var info = document.createElement('div');
      info.className = 'tx-log-info';

      var title = document.createElement('div');
      title.className = 'tx-log-title';
      var titleParts = [];
      if (tx.item_name) titleParts.push(tx.item_name);
      if (tx.quantity > 1) titleParts.push('x' + tx.quantity);
      title.textContent = titleParts.join(' ') || 'Transaction';
      info.appendChild(title);

      var meta = document.createElement('div');
      meta.className = 'tx-log-meta';
      var metaParts = [];
      metaParts.push(capitalize(tx.transaction_type || 'unknown'));
      if (tx.buyer_name) metaParts.push('by ' + tx.buyer_name);
      meta.textContent = metaParts.join(' \u2022 ');
      info.appendChild(meta);

      row.appendChild(info);

      // Price.
      if (tx.price_paid) {
        var price = document.createElement('span');
        price.className = 'tx-log-price';
        price.textContent = tx.price_paid;
        row.appendChild(price);
      }

      // Date.
      var date = document.createElement('span');
      date.className = 'tx-log-date';
      date.textContent = formatDate(tx.created_at);
      row.appendChild(date);

      return row;
    }

    // --- API ---

    function loadTransactions(append) {
      if (!append) state.loading = true;
      render();

      Chronicle.apiFetch(txEndpoint + '?page=' + state.page + '&per_page=10')
        .then(function (res) { return res.json(); })
        .then(function (data) {
          var items = data.data || [];
          if (append) {
            state.transactions = state.transactions.concat(items);
          } else {
            state.transactions = items;
          }
          state.total = data.total || 0;
          state.loading = false;
          render();
        })
        .catch(function (err) {
          console.error('Transaction log: failed to load', err);
          state.loading = false;
          render();
        });
    }

    // --- Helpers ---

    function capitalize(s) {
      return s.charAt(0).toUpperCase() + s.slice(1);
    }

    function formatDate(isoStr) {
      if (!isoStr) return '';
      try {
        var d = new Date(isoStr);
        return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
      } catch (e) {
        return '';
      }
    }

    // --- Init ---
    loadTransactions();
  },

  destroy: function (el) {
    el.innerHTML = '';
    delete el._txState;
  }
});
