/**
 * editor_image.js -- pictures inside editor text.
 *
 * Exposes Chronicle.EditorImage:
 *   .extension            TipTap node "chronicleImage" (a block picture)
 *   .pickAndInsert(ed, campaignId)          file chooser -> upload -> insert
 *   .uploadAndInsert(ed, campaignId, files, pos)
 *   .imageFiles(dataTransfer)               the image files in a paste/drop
 *   .useNotePictures(ed, campaignId)        send this editor's pictures to the
 *                                           notes picture route (players may)
 *   .pasteDropProps(ed, campaignId)         editorProps that upload pasted or
 *                                           dropped picture files
 *
 * Saved HTML is <figure class="ce-img ce-img--w40 ce-img--right
 * [ce-img--gm]"><img src="/media/<id>" alt><figcaption>…</figcaption></figure>.
 * Everything lives in classes and a plain /media/<id> src because the shared
 * sanitizer keeps `class` and relative image URLs but strips style and most
 * data-* attributes. The src must stay the plain path: the Foundry frames
 * swap it for a short-lived signed link on screen, and a signed link saved
 * into a page would expire. getHTML() renders from node attributes, never
 * from the DOM, and the node view ignores DOM mutations, so a swapped src
 * cannot leak into a save.
 *
 * ce-img--gm marks a GM-only picture; the server drops it (and its caption)
 * for players in sanitize.StripSecretsHTML/StripSecretsJSON.
 */
(function () {
  'use strict';

  if (!window.TipTap || !TipTap.Node) return;

  var MIN_WIDTH = 10;
  var MAX_WIDTH = 100;
  var STEP = 5;
  var ALIGNS = ['left', 'center', 'right'];
  var MEDIA_ID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
  var MAX_CAPTION = 300;

  function clampWidth(w) {
    w = Math.round(Number(w) / STEP) * STEP;
    if (!isFinite(w)) return MAX_WIDTH;
    return Math.max(MIN_WIDTH, Math.min(MAX_WIDTH, w));
  }

  // mediaIdFromSrc accepts only this site's own media path, so a pasted
  // picture from another website is never stored as a hotlink.
  function mediaIdFromSrc(src) {
    var m = /^\/media\/([^/?#]+)/.exec(String(src || ''));
    return m && MEDIA_ID_RE.test(m[1]) ? m[1] : null;
  }

  function classesFor(attrs) {
    var cls = ['ce-img', 'ce-img--w' + clampWidth(attrs.width), 'ce-img--' + (ALIGNS.indexOf(attrs.align) >= 0 ? attrs.align : 'center')];
    if (attrs.gmOnly) cls.push('ce-img--gm');
    return cls.join(' ');
  }

  // attrsFromFigure reads a saved figure back into node attributes.
  function attrsFromFigure(fig) {
    var img = fig.querySelector('img');
    var id = img && mediaIdFromSrc(img.getAttribute('src'));
    if (!id) return false;
    var cls = ' ' + (fig.getAttribute('class') || '') + ' ';
    var w = /\sce-img--w(\d+)\s/.exec(cls);
    var align = 'center';
    ALIGNS.forEach(function (a) { if (cls.indexOf(' ce-img--' + a + ' ') >= 0) align = a; });
    var cap = fig.querySelector('figcaption');
    return {
      mediaId: id,
      alt: img.getAttribute('alt') || '',
      caption: cap ? cap.textContent.slice(0, MAX_CAPTION) : '',
      width: w ? clampWidth(w[1]) : MAX_WIDTH,
      align: align,
      gmOnly: cls.indexOf(' ce-img--gm ') >= 0,
    };
  }

  function renderSpec(attrs) {
    var spec = ['figure', { class: classesFor(attrs) },
      ['img', { src: '/media/' + attrs.mediaId, alt: attrs.alt || '' }]];
    if (attrs.caption) spec.push(['figcaption', {}, attrs.caption]);
    return spec;
  }

  // --- Node view (editing UI) -------------------------------------------

  function button(label, icon, title) {
    var b = document.createElement('button');
    b.type = 'button';
    b.className = 'ce-img__btn';
    b.title = title || label;
    b.setAttribute('aria-label', title || label);
    b.innerHTML = icon ? '<i class="fa-solid ' + icon + '" aria-hidden="true"></i>' : '';
    if (!icon) b.textContent = label;
    return b;
  }

  function createNodeView(props) {
    var node = props.node;
    var editor = props.editor;
    var getPos = props.getPos;

    var fig = document.createElement('figure');
    var img = document.createElement('img');
    img.draggable = false;
    var cap = document.createElement('figcaption');
    fig.appendChild(img);
    fig.appendChild(cap);

    // Tools show only while editing and the picture is selected (CSS).
    var tools = document.createElement('div');
    tools.className = 'ce-img__tools';
    tools.setAttribute('contenteditable', 'false');
    var alignBtns = {
      left: button('Left', 'fa-align-left', 'Wrap text on the right'),
      center: button('Centre', 'fa-align-center', 'Centre, no wrap'),
      right: button('Right', 'fa-align-right', 'Wrap text on the left'),
    };
    ALIGNS.forEach(function (a) { tools.appendChild(alignBtns[a]); });
    var capInput = document.createElement('input');
    capInput.type = 'text';
    capInput.className = 'ce-img__caption-input';
    capInput.placeholder = 'Caption';
    capInput.maxLength = MAX_CAPTION;
    capInput.setAttribute('aria-label', 'Caption');
    tools.appendChild(capInput);
    var gmBtn = button('GM only', null, 'Hide this picture from players');
    gmBtn.classList.add('ce-img__btn--gm');
    tools.appendChild(gmBtn);
    var delBtn = button('Remove', 'fa-trash', 'Remove picture');
    tools.appendChild(delBtn);
    fig.appendChild(tools);

    var handle = document.createElement('span');
    handle.className = 'ce-img__handle';
    handle.setAttribute('role', 'slider');
    handle.setAttribute('tabindex', '0');
    handle.setAttribute('aria-label', 'Picture width');
    handle.setAttribute('aria-valuemin', String(MIN_WIDTH));
    handle.setAttribute('aria-valuemax', String(MAX_WIDTH));
    fig.appendChild(handle);

    var gmBadge = document.createElement('span');
    gmBadge.className = 'ce-img__gm-badge';
    gmBadge.textContent = 'GM only';
    fig.appendChild(gmBadge);

    function paint() {
      var a = node.attrs;
      fig.className = classesFor(a) + (fig.classList.contains('is-selected') ? ' is-selected' : '');
      fig.style.width = '';
      var src = '/media/' + a.mediaId;
      // Compare against the attribute, not .src, so a frame that swapped in
      // a signed link isn't fought on every redraw.
      if (img.getAttribute('data-src') !== src) {
        img.setAttribute('data-src', src);
        img.setAttribute('src', src);
      }
      img.alt = a.alt || '';
      cap.textContent = a.caption || '';
      cap.hidden = !a.caption;
      if (document.activeElement !== capInput) capInput.value = a.caption || '';
      ALIGNS.forEach(function (al) { alignBtns[al].classList.toggle('is-on', a.align === al); });
      // Read per paint: the notes editors flag themselves after they are built.
      gmBtn.style.display = editor.chronicleNotePictures ? 'none' : '';
      gmBtn.classList.toggle('is-on', !!a.gmOnly);
      gmBtn.setAttribute('aria-pressed', a.gmOnly ? 'true' : 'false');
      handle.setAttribute('aria-valuenow', String(clampWidth(a.width)));
    }

    function setAttrs(patch) {
      if (typeof getPos !== 'function') return;
      var pos = getPos();
      if (typeof pos !== 'number') return;
      var next = {};
      for (var k in node.attrs) next[k] = node.attrs[k];
      for (var p in patch) next[p] = patch[p];
      editor.view.dispatch(editor.view.state.tr.setNodeMarkup(pos, undefined, next));
    }

    ALIGNS.forEach(function (a) {
      alignBtns[a].addEventListener('click', function () { setAttrs({ align: a }); });
    });
    gmBtn.addEventListener('click', function () { setAttrs({ gmOnly: !node.attrs.gmOnly }); });
    delBtn.addEventListener('click', function () {
      var pos = getPos();
      if (typeof pos !== 'number') return;
      editor.view.dispatch(editor.view.state.tr.delete(pos, pos + node.nodeSize));
      editor.commands.focus();
    });
    function commitCaption() {
      var v = capInput.value.trim().slice(0, MAX_CAPTION);
      if (v !== (node.attrs.caption || '')) setAttrs({ caption: v });
    }
    capInput.addEventListener('change', commitCaption);
    capInput.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') { e.preventDefault(); commitCaption(); editor.commands.focus(); }
    });

    // Drag the corner to any width. Measured against the text column, so a
    // width means the same on every screen; phones show every picture full
    // width regardless (CSS).
    function widthFromPointer(clientX) {
      var parent = fig.parentNode;
      var pw = parent ? parent.getBoundingClientRect().width : 0;
      if (!pw) return node.attrs.width;
      var r = fig.getBoundingClientRect();
      var px;
      if (node.attrs.align === 'right') px = r.right - clientX;
      else if (node.attrs.align === 'center') px = 2 * (clientX - (r.left + r.width / 2));
      else px = clientX - r.left;
      return (px / pw) * 100;
    }
    handle.addEventListener('pointerdown', function (e) {
      if (!editor.isEditable) return;
      e.preventDefault();
      handle.setPointerCapture(e.pointerId);
      var live = node.attrs.width;
      function move(ev) {
        live = Math.max(MIN_WIDTH, Math.min(MAX_WIDTH, widthFromPointer(ev.clientX)));
        fig.style.width = live + '%';
      }
      function up() {
        handle.removeEventListener('pointermove', move);
        handle.removeEventListener('pointerup', up);
        handle.removeEventListener('pointercancel', up);
        fig.style.width = '';
        setAttrs({ width: clampWidth(live) });
      }
      handle.addEventListener('pointermove', move);
      handle.addEventListener('pointerup', up);
      handle.addEventListener('pointercancel', up);
    });
    handle.addEventListener('keydown', function (e) {
      var d = e.key === 'ArrowRight' || e.key === 'ArrowUp' ? STEP : (e.key === 'ArrowLeft' || e.key === 'ArrowDown' ? -STEP : 0);
      if (!d) return;
      e.preventDefault();
      setAttrs({ width: clampWidth(node.attrs.width + d) });
    });

    paint();

    return {
      dom: fig,
      update: function (updated) {
        if (updated.type !== node.type) return false;
        node = updated;
        paint();
        return true;
      },
      selectNode: function () { gmBtn.style.display = editor.chronicleNotePictures ? 'none' : ''; fig.classList.add('is-selected'); },
      deselectNode: function () { fig.classList.remove('is-selected'); commitCaption(); },
      // Clicks and typing in the tools stay out of ProseMirror.
      stopEvent: function (e) {
        return tools.contains(e.target) || e.target === handle;
      },
      // The picture's DOM is ours (and a Foundry frame may rewrite its src);
      // ProseMirror must not re-read it.
      ignoreMutation: function () { return true; },
    };
  }

  var ImageNode = TipTap.Node.create({
    name: 'chronicleImage',
    group: 'block',
    atom: true,
    draggable: true,
    selectable: true,

    addAttributes: function () {
      return {
        mediaId: { default: null },
        alt: { default: '' },
        caption: { default: '' },
        width: { default: MAX_WIDTH },
        align: { default: 'center' },
        gmOnly: { default: false },
      };
    },

    parseHTML: function () {
      return [{ tag: 'figure.ce-img', getAttrs: attrsFromFigure }];
    },

    renderHTML: function (props) {
      return renderSpec(props.node.attrs);
    },

    addNodeView: function () {
      return createNodeView;
    },
  });

  // --- Upload and insert --------------------------------------------------

  function imageFiles(dt) {
    var out = [];
    if (!dt || !dt.files) return out;
    for (var i = 0; i < dt.files.length; i++) {
      if (/^image\//.test(dt.files[i].type)) out.push(dt.files[i]);
    }
    return out;
  }

  function post(url, file, fields) {
    var body = new FormData();
    body.append('file', file);
    for (var k in fields) body.append(k, fields[k]);
    return Chronicle.apiFetch(url, { method: 'POST', body: body })
      .then(function (res) {
        if (!res.ok) {
          return res.json().catch(function () { return {}; }).then(function (j) {
            throw new Error((j && (j.message || j.error)) || 'Upload failed');
          });
        }
        return res.json();
      });
  }

  // Pages: the campaign's media library (Scribe and up).
  function upload(campaignId, file) {
    return post('/media/upload', file, { campaign_id: campaignId, usage_type: 'attachment' });
  }

  // Notes: any member may add a picture to their own note. The server keeps
  // the file private to the readers of the note that holds it, and the same
  // address works in the Foundry notebook frame, which has no media-library
  // access.
  function uploadNotePicture(campaignId, file) {
    return post('/campaigns/' + encodeURIComponent(campaignId) + '/notes/pictures', file, {});
  }

  // useNotePictures makes this editor's pictures go to the notes route and
  // drops the "GM only" switch, which notes have no meaning for: a note's
  // audience is its own share setting.
  function useNotePictures(editor, campaignId) {
    editor.chronicleImageUpload = function (file) { return uploadNotePicture(campaignId, file); };
    editor.chronicleNotePictures = true;
  }

  function uploadAndInsert(editor, campaignId, files, pos) {
    if (!campaignId || !files || !files.length) return Promise.resolve(0);
    var done = 0;
    var chain = Promise.resolve();
    files.forEach(function (file) {
      chain = chain.then(function () {
        var send = editor.chronicleImageUpload || function (f) { return upload(campaignId, f); };
        return send(file).then(function (data) {
          if (!data || !MEDIA_ID_RE.test(String(data.id || ''))) throw new Error('Upload failed');
          var content = { type: 'chronicleImage', attrs: { mediaId: data.id, alt: (file.name || '').replace(/\.[^.]+$/, '').slice(0, 120) } };
          if (typeof pos === 'number') {
            editor.chain().focus().insertContentAt(pos, content).run();
          } else {
            editor.chain().focus().insertContent(content).run();
          }
          done++;
        });
      });
    });
    return chain.then(function () { return done; }, function (err) {
      if (Chronicle.notify) Chronicle.notify(err && err.message ? err.message : 'Upload failed', 'error');
      return done;
    });
  }

  function pickAndInsert(editor, campaignId) {
    if (!campaignId) return;
    var input = document.createElement('input');
    input.type = 'file';
    input.accept = 'image/png,image/jpeg,image/webp,image/gif';
    input.multiple = true;
    input.addEventListener('change', function () {
      var files = [];
      for (var i = 0; i < input.files.length; i++) files.push(input.files[i]);
      uploadAndInsert(editor, campaignId, files);
    });
    input.click();
  }

  // pasteDropProps are editorProps for an editor that takes picture files
  // pasted or dropped into the text. editor is a getter because the props are
  // needed to build the editor they act on.
  function pasteDropProps(getEditor, campaignId) {
    function take(view, files, pos) {
      var ed = getEditor();
      if (!ed || !files.length || !view.editable) return false;
      uploadAndInsert(ed, campaignId, files, pos);
      return true;
    }
    return {
      handlePaste: function (view, event) {
        return take(view, imageFiles(event.clipboardData));
      },
      handleDrop: function (view, event, slice, moved) {
        if (moved) return false;
        var files = imageFiles(event.dataTransfer);
        if (!files.length) return false;
        event.preventDefault();
        var at = view.posAtCoords({ left: event.clientX, top: event.clientY });
        return take(view, files, at ? at.pos : undefined);
      }
    };
  }

  // "/picture" in the slash menu (loaded before this file).
  if (window.Chronicle && Chronicle.SlashCommands && Chronicle.SlashCommands.addCommand) {
    Chronicle.SlashCommands.addCommand({
      id: 'picture', label: 'Picture', icon: 'fa-image',
      keywords: 'picture image photo map portrait handout',
      description: 'Upload a picture into the text',
      customHandler: function (editor) { pickAndInsert(editor, editor.chronicleCampaignId); },
    });
  }

  window.Chronicle = window.Chronicle || {};
  window.Chronicle.EditorImage = {
    extension: ImageNode,
    pickAndInsert: pickAndInsert,
    uploadAndInsert: uploadAndInsert,
    imageFiles: imageFiles,
    useNotePictures: useNotePictures,
    pasteDropProps: pasteDropProps,
    // Exposed for tests.
    _clampWidth: clampWidth,
    _mediaIdFromSrc: mediaIdFromSrc,
    _classesFor: classesFor,
    _renderSpec: renderSpec,
  };
})();
