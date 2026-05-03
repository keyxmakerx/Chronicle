/**
 * entity_posts.js -- Chronicle Entity Posts (Sub-Notes) Widget
 *
 * Displays and manages sub-notes/posts attached to an entity. Each post
 * has a name, TipTap rich text content, visibility toggle, and reorder support.
 * Auto-mounted by boot.js on elements with data-widget="entity-posts".
 *
 * Config (from data-* attributes):
 *   data-entity-id     - Entity ID
 *   data-campaign-id   - Campaign ID
 *   data-endpoint      - Posts API endpoint (GET/POST /campaigns/:id/entities/:eid/posts)
 *   data-editable      - "true" if user can create/edit/delete posts
 *   data-csrf          - CSRF token for mutations
 */
(function () {
  'use strict';

  Chronicle.register('entity-posts', {
    init: function (el, config) {
      var endpoint = config.endpoint || '';
      var campaignId = config.campaignId || '';
      var entityId = config.entityId || '';
      var editable = config.editable === true;
      var csrf = config.csrf || '';

      var state = {
        posts: [],
        loading: true,
        expandedPostId: null,
        editingPostId: null
      };

      // --- Load Posts ---

      function loadPosts() {
        state.loading = true;
        render();

        Chronicle.apiFetch(endpoint)
          .then(function (posts) {
            state.posts = posts || [];
            state.loading = false;
            render();
          })
          .catch(function (err) {
            state.loading = false;
            console.error('[EntityPosts] Load error:', err);
            render();
          });
      }

      // --- Create Post ---

      function createPost(name) {
        Chronicle.apiFetch(endpoint, {
          method: 'POST',
          body: JSON.stringify({ name: name, isPrivate: false })
        })
          .then(function (post) {
            state.posts.push(post);
            state.expandedPostId = post.id;
            render();
          })
          .catch(function (err) {
            console.error('[EntityPosts] Create error:', err);
            Chronicle.notify('Failed to create post', 'error');
          });
      }

      // --- Update Post ---

      function updatePost(postId, data) {
        Chronicle.apiFetch(endpoint + '/' + postId, {
          method: 'PUT',
          body: JSON.stringify(data)
        })
          .then(function (updated) {
            for (var i = 0; i < state.posts.length; i++) {
              if (state.posts[i].id === postId) {
                state.posts[i] = updated;
                break;
              }
            }
            render();
          })
          .catch(function (err) {
            console.error('[EntityPosts] Update error:', err);
            Chronicle.notify('Failed to update post', 'error');
          });
      }

      // --- Delete Post ---

      function deletePost(postId) {
        Chronicle.apiFetch(endpoint + '/' + postId, { method: 'DELETE' })
          .then(function () {
            state.posts = state.posts.filter(function (p) { return p.id !== postId; });
            if (state.expandedPostId === postId) state.expandedPostId = null;
            render();
          })
          .catch(function (err) {
            console.error('[EntityPosts] Delete error:', err);
            Chronicle.notify('Failed to delete post', 'error');
          });
      }

      // --- Reorder Posts ---

      function reorderPosts(postIds) {
        Chronicle.apiFetch(endpoint + '/reorder', {
          method: 'PUT',
          body: JSON.stringify({ postIds: postIds })
        })
          .catch(function (err) {
            console.error('[EntityPosts] Reorder error:', err);
          });
      }

      // --- Render ---

      function render() {
        if (state.loading) {
          el.innerHTML = '<div class="text-sm text-fg-muted py-4">Loading posts...</div>';
          return;
        }

        var html = '';

        // Header with create button.
        html += '<div class="flex items-center justify-between mb-3">';
        html += '<h3 class="text-sm font-semibold text-fg-secondary uppercase tracking-wider">';
        html += '<i class="fa-solid fa-layer-group mr-1.5"></i>Posts';
        if (state.posts.length > 0) {
          html += ' <span class="text-xs font-normal text-fg-muted">(' + state.posts.length + ')</span>';
        }
        html += '</h3>';
        if (editable) {
          html += '<button class="btn-secondary text-xs" data-action="create-post">';
          html += '<i class="fa-solid fa-plus mr-1"></i>Add Post';
          html += '</button>';
        }
        html += '</div>';

        // Post list.
        if (state.posts.length === 0) {
          html += '<div class="card p-6 text-center">';
          html += '<i class="fa-solid fa-layer-group text-2xl text-fg-muted mb-2"></i>';
          html += '<p class="text-sm text-fg-muted">No posts yet.';
          if (editable) {
            html += ' Add sub-notes to organize additional content for this page.';
          }
          html += '</p></div>';
        } else {
          html += '<div class="space-y-2">';
          for (var i = 0; i < state.posts.length; i++) {
            html += renderPost(state.posts[i], i);
          }
          html += '</div>';
        }

        el.innerHTML = html;
        bindEvents();
      }

      function renderPost(post, index) {
        var isExpanded = state.expandedPostId === post.id;
        var h = '';

        h += '<div class="card overflow-hidden" data-post-id="' + esc(post.id) + '"';
        if (editable) {
          h += ' draggable="true" data-post-index="' + index + '"';
        }
        h += '>';

        // Post header (always visible, clickable to expand/collapse).
        h += '<div class="px-4 py-3 flex items-center gap-2 cursor-pointer select-none hover:bg-surface-alt/50 transition-colors" data-action="toggle-post" data-post-id="' + esc(post.id) + '">';

        // Drag handle.
        if (editable) {
          h += '<span class="text-fg-muted cursor-grab" title="Drag to reorder"><i class="fa-solid fa-grip-vertical text-xs"></i></span>';
        }

        // Expand chevron.
        h += '<i class="fa-solid fa-chevron-' + (isExpanded ? 'down' : 'right') + ' text-xs text-fg-muted w-3"></i>';

        // Post name.
        h += '<span class="text-sm font-medium text-fg flex-1">' + esc(post.name) + '</span>';

        // Badges.
        if (post.isPrivate) {
          h += '<span class="text-xs text-amber-500" title="DM only"><i class="fa-solid fa-lock"></i></span>';
        }

        // Actions dropdown.
        if (editable) {
          h += '<div class="relative" data-dropdown>';
          h += '<button class="text-fg-muted hover:text-fg text-xs p-1" data-action="post-menu" data-post-id="' + esc(post.id) + '" title="More actions"><i class="fa-solid fa-ellipsis-vertical"></i></button>';
          h += '<div class="hidden absolute right-0 top-full mt-1 bg-surface border border-edge rounded-lg shadow-lg py-1 z-20 min-w-[140px]" data-menu="' + esc(post.id) + '">';
          h += '<button class="w-full text-left px-3 py-1.5 text-xs hover:bg-surface-alt" data-action="rename-post" data-post-id="' + esc(post.id) + '"><i class="fa-solid fa-pen mr-2"></i>Rename</button>';
          h += '<button class="w-full text-left px-3 py-1.5 text-xs hover:bg-surface-alt" data-action="toggle-private" data-post-id="' + esc(post.id) + '"><i class="fa-solid fa-' + (post.isPrivate ? 'unlock' : 'lock') + ' mr-2"></i>' + (post.isPrivate ? 'Make Public' : 'Make DM Only') + '</button>';
          h += '<button class="w-full text-left px-3 py-1.5 text-xs text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20" data-action="delete-post" data-post-id="' + esc(post.id) + '"><i class="fa-solid fa-trash mr-2"></i>Delete</button>';
          h += '</div></div>';
        }

        h += '</div>';

        // Expanded content area.
        if (isExpanded) {
          h += '<div class="border-t border-edge">';
          if (post.entryHtml) {
            // entryHtml is sanitized server-side via sanitize.HTML() before storage.
            h += '<div class="px-4 py-3 prose prose-sm dark:prose-invert max-w-none">' + post.entryHtml + '</div>';
          } else {
            h += '<div class="px-4 py-3 text-sm text-fg-muted italic">No content yet.</div>';
          }

          // Editor mount point for editable posts.
          if (editable) {
            h += '<div class="border-t border-edge px-4 py-2">';
            h += '<div data-post-editor="' + esc(post.id) + '"';
            h += ' data-widget="editor"';
            h += ' data-endpoint="' + esc(endpoint + '/' + post.id) + '"';
            h += ' data-campaign-id="' + esc(campaignId) + '"';
            h += ' data-editable="true"';
            h += ' data-autosave="30"';
            h += ' data-csrf-token="' + esc(csrf) + '"';
            h += ' data-compact="true"';
            h += '></div>';
            h += '</div>';
          }

          h += '</div>';
        }

        h += '</div>';
        return h;
      }

      // --- Event Binding ---

      function bindEvents() {
        // Create post.
        var createBtn = el.querySelector('[data-action="create-post"]');
        if (createBtn) {
          createBtn.addEventListener('click', function (e) {
            e.stopPropagation();
            var name = prompt('Post name:');
            if (name && name.trim()) {
              createPost(name.trim());
            }
          });
        }

        // Toggle expand/collapse.
        var toggleBtns = el.querySelectorAll('[data-action="toggle-post"]');
        for (var i = 0; i < toggleBtns.length; i++) {
          toggleBtns[i].addEventListener('click', function (e) {
            var postId = this.getAttribute('data-post-id');
            // Don't toggle if clicking on a button inside.
            if (e.target.closest('[data-action="post-menu"]') || e.target.closest('[data-dropdown]')) return;
            state.expandedPostId = state.expandedPostId === postId ? null : postId;
            render();

            // Auto-mount any new editor widgets after re-render.
            if (state.expandedPostId && window.Chronicle && Chronicle.mountWidgets) {
              setTimeout(function () { Chronicle.mountWidgets(el); }, 50);
            }
          });
        }

        // Menu toggles.
        var menuBtns = el.querySelectorAll('[data-action="post-menu"]');
        for (var j = 0; j < menuBtns.length; j++) {
          menuBtns[j].addEventListener('click', function (e) {
            e.stopPropagation();
            var postId = this.getAttribute('data-post-id');
            var menu = el.querySelector('[data-menu="' + postId + '"]');
            if (menu) {
              // Close other menus.
              var allMenus = el.querySelectorAll('[data-menu]');
              for (var k = 0; k < allMenus.length; k++) {
                if (allMenus[k] !== menu) allMenus[k].classList.add('hidden');
              }
              menu.classList.toggle('hidden');
            }
          });
        }

        // Rename post.
        var renameBtns = el.querySelectorAll('[data-action="rename-post"]');
        for (var m = 0; m < renameBtns.length; m++) {
          renameBtns[m].addEventListener('click', function (e) {
            e.stopPropagation();
            var postId = this.getAttribute('data-post-id');
            var post = findPost(postId);
            if (!post) return;
            var name = prompt('Rename post:', post.name);
            if (name && name.trim() && name.trim() !== post.name) {
              updatePost(postId, { name: name.trim() });
            }
          });
        }

        // Toggle private.
        var privateBtns = el.querySelectorAll('[data-action="toggle-private"]');
        for (var n = 0; n < privateBtns.length; n++) {
          privateBtns[n].addEventListener('click', function (e) {
            e.stopPropagation();
            var postId = this.getAttribute('data-post-id');
            var post = findPost(postId);
            if (!post) return;
            updatePost(postId, { isPrivate: !post.isPrivate });
          });
        }

        // Delete post.
        var deleteBtns = el.querySelectorAll('[data-action="delete-post"]');
        for (var p = 0; p < deleteBtns.length; p++) {
          deleteBtns[p].addEventListener('click', function (e) {
            e.stopPropagation();
            var postId = this.getAttribute('data-post-id');
            if (confirm('Delete this post? This cannot be undone.')) {
              deletePost(postId);
            }
          });
        }

        // Close menus on outside click.
        document.addEventListener('click', function () {
          var allMenus = el.querySelectorAll('[data-menu]');
          for (var q = 0; q < allMenus.length; q++) {
            allMenus[q].classList.add('hidden');
          }
        });

        // Drag and drop reorder.
        if (editable) {
          bindDragDrop();
        }
      }

      // --- Drag & Drop ---

      function bindDragDrop() {
        var cards = el.querySelectorAll('[draggable="true"]');
        var dragSrcIndex = null;

        for (var i = 0; i < cards.length; i++) {
          cards[i].addEventListener('dragstart', function (e) {
            dragSrcIndex = parseInt(this.getAttribute('data-post-index'), 10);
            this.style.opacity = '0.5';
            e.dataTransfer.effectAllowed = 'move';
          });

          cards[i].addEventListener('dragover', function (e) {
            e.preventDefault();
            e.dataTransfer.dropEffect = 'move';
            this.classList.add('ring-2', 'ring-accent');
          });

          cards[i].addEventListener('dragleave', function () {
            this.classList.remove('ring-2', 'ring-accent');
          });

          cards[i].addEventListener('drop', function (e) {
            e.preventDefault();
            this.classList.remove('ring-2', 'ring-accent');
            var dropIndex = parseInt(this.getAttribute('data-post-index'), 10);
            if (dragSrcIndex === null || dragSrcIndex === dropIndex) return;

            // Reorder in local state.
            var moved = state.posts.splice(dragSrcIndex, 1)[0];
            state.posts.splice(dropIndex, 0, moved);

            // Send reorder to API.
            var postIds = state.posts.map(function (p) { return p.id; });
            reorderPosts(postIds);

            render();
          });

          cards[i].addEventListener('dragend', function () {
            this.style.opacity = '1';
          });
        }
      }

      // --- Helpers ---

      function findPost(id) {
        for (var i = 0; i < state.posts.length; i++) {
          if (state.posts[i].id === id) return state.posts[i];
        }
        return null;
      }

      function esc(s) {
        return Chronicle.escapeHtml ? Chronicle.escapeHtml(s || '') : (s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
      }

      // --- Init ---
      loadPosts();
    },

    destroy: function (el) {
      el.innerHTML = '';
    }
  });
})();
