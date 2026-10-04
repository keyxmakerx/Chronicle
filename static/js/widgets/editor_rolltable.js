/**
 * editor_rolltable.js -- a rolling-table roller inside a page.
 *
 * Exposes Chronicle.EditorRollTable:
 *   .extension   TipTap node "rollTable" (a block roller), placed with /roll
 *
 * Saved HTML is <div class="ce-roll ce-roll--t-<table> ce-roll--n-<count>">
 * </div>: the table and count live in classes because the shared sanitizer
 * keeps `class` but strips most data-* attributes. The roller is a DM tool,
 * so the server drops the node for players (sanitize.StripSecretsHTML and
 * StripSecretsJSON); what reaches players is only what "Put in page" writes
 * into the text. Rolling never changes the page; picking a table or a count
 * is kept only while the page is being edited.
 */
(function () {
  'use strict';

  if (!window.TipTap || !TipTap.Node) return;

  var TABLE_RE = /^[a-z][a-z0-9-]{0,63}$/;
  var COUNTS = [1, 3, 5];

  function cleanTable(t) { return TABLE_RE.test(String(t || '')) ? String(t) : 'rumours'; }
  function cleanCount(n) { n = +n; return COUNTS.indexOf(n) >= 0 ? n : 3; }

  function attrsFromDiv(el) {
    var a = { table: 'rumours', count: 3 };
    (el.getAttribute('class') || '').split(/\s+/).forEach(function (c) {
      var m = /^ce-roll--t-(.+)$/.exec(c);
      if (m) a.table = cleanTable(m[1]);
      m = /^ce-roll--n-(\d+)$/.exec(c);
      if (m) a.count = cleanCount(m[1]);
    });
    return a;
  }
  function renderSpec(attrs) {
    return ['div', { class: 'ce-roll ce-roll--t-' + cleanTable(attrs.table) + ' ce-roll--n-' + cleanCount(attrs.count) }];
  }

  /* "Put in page" writes the results as ordinary text just above the roller:
     a bold name and its line, one paragraph each. */
  function resultContent(rows) {
    return rows.map(function (r) {
      var c = [{ type: 'text', text: r.name, marks: [{ type: 'bold' }] }];
      if (r.brief) c.push({ type: 'text', text: ' ' + r.brief });
      return { type: 'paragraph', content: c };
    });
  }

  function createNodeView(props) {
    var node = props.node, editor = props.editor, getPos = props.getPos;
    var dom = document.createElement('div');
    dom.className = 'ce-roll';
    dom.setAttribute('contenteditable', 'false');
    var roller = null;
    if (window.Chronicle && Chronicle.RollTables) {
      roller = Chronicle.RollTables.mountBlock(dom, {
        campaignId: editor.chronicleCampaignId || '',
        table: node.attrs.table,
        count: node.attrs.count,
        onAttrs: function (table, count) {
          if (!editor.isEditable || typeof getPos !== 'function') return;
          var pos = getPos();
          if (typeof pos !== 'number') return;
          editor.view.dispatch(editor.view.state.tr.setNodeMarkup(pos, undefined, { table: table, count: count }));
        },
        onPut: function (rows) {
          var put = function () {
            var pos = typeof getPos === 'function' ? getPos() : null;
            if (typeof pos !== 'number') return;
            editor.chain().insertContentAt(pos, resultContent(rows)).run();
            if (Chronicle.notify) Chronicle.notify('Put in the page. Press Done to save it.', 'success');
          };
          if (editor.isEditable) return put();
          // Reading: the page has to be in edit mode to take new text.
          var host = dom.closest('.chronicle-editor');
          var btn = host && host.querySelector('.chronicle-editor__edit-btn');
          if (btn) btn.click();
          setTimeout(put, 0);
        }
      });
    } else {
      dom.textContent = 'Rolling table';
    }
    return {
      dom: dom,
      update: function (updated) { if (updated.type !== node.type) return false; node = updated; return true; },
      stopEvent: function () { return true; },
      ignoreMutation: function () { return true; },
      destroy: function () { if (roller) roller.destroy(); }
    };
  }

  var RollTableNode = TipTap.Node.create({
    name: 'rollTable',
    group: 'block',
    atom: true,
    draggable: true,
    selectable: true,
    addAttributes: function () {
      return { table: { default: 'rumours' }, count: { default: 3 } };
    },
    parseHTML: function () {
      return [{ tag: 'div.ce-roll', getAttrs: attrsFromDiv }];
    },
    renderHTML: function (props) { return renderSpec(props.node.attrs); },
    addNodeView: function () { return createNodeView; }
  });

  // "/roll" in the slash menu (loaded before this file).
  if (window.Chronicle && Chronicle.SlashCommands && Chronicle.SlashCommands.addCommand) {
    Chronicle.SlashCommands.addCommand({
      id: 'rollTable', label: 'Rolling table', icon: 'fa-dice-d20',
      keywords: 'roll table random rumour omen encounter name generator dice',
      description: 'Roll rumours, omens, names and more',
      customHandler: function (ed) { ed.chain().focus().insertContent({ type: 'rollTable', attrs: { table: 'rumours', count: 3 } }).run(); }
    });
  }

  window.Chronicle = window.Chronicle || {};
  window.Chronicle.EditorRollTable = {
    extension: RollTableNode,
    // Exposed for tests.
    _attrsFromDiv: attrsFromDiv,
    _renderSpec: renderSpec,
    _resultContent: resultContent
  };
})();
