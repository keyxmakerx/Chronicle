#!/usr/bin/env bash
# tools/check-escape-helpers.sh
#
# Front-end scripts escape HTML through Chronicle.escapeHtml (static/js/boot.js),
# not their own copies: copies drift (some skipped the single quote) and each
# one is another place to get escaping wrong. This fails on any script that
# spells out an escape table ('&amp;' as a replacement string) except the files
# listed below.
#
# Allowed: boot.js defines the helper. The shop room and crafting library files
# are also copied unchanged into the Foundry module, which has no boot.js, and
# the other four load without Chronicle (pure logic run in Node, or helpers a
# page reads before boot.js), so they keep their own.
#
# Exit: 0 clean / 1 a script carries its own escape table.

set -euo pipefail

allowed=(
  static/js/boot.js
  static/js/widgets/shop_room.js
  static/js/widgets/shop_room_counter.js
  static/js/widgets/shop_room_paper.js
  internal/plugins/crafting/static/js/crafting_lib.js
  internal/plugins/pageblocks/static/js/page_map.js
  static/js/hovercard.js
  static/js/widgets/craft_rules_logic.js
  static/js/widgets/roll_tables.js
)

hits=$(git ls-files 'static/js/*.js' 'internal/*.js' \
  | grep -v -e '_test\.js$' -e '/vendor/' \
  | xargs grep -nE "['\"]&amp;['\"]" 2>/dev/null || true)

bad=""
while IFS= read -r line; do
  [ -z "$line" ] && continue
  file=${line%%:*}
  skip=0
  for a in "${allowed[@]}"; do [ "$file" = "$a" ] && skip=1; done
  [ "$skip" = 0 ] && bad+="$line"$'\n'
done <<<"$hits"

if [ -n "$bad" ]; then
  echo "ERROR: these scripts carry their own HTML escape; call Chronicle.escapeHtml instead:"
  printf '%s' "$bad"
  exit 1
fi
echo "check-escape-helpers: OK"
