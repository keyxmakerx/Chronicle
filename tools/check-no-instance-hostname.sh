#!/usr/bin/env bash
# tools/check-no-instance-hostname.sh
#
# Operator-security guard: fail if tracked source names the operator's
# production hostname, so a future PR can't reintroduce it.
#
# It holds only a SHA-256 fingerprint of the hostname's distinctive label,
# never the label itself: a guard that spells out what it guards publishes
# it. Every word in tracked text files is hashed and compared, and a hit is
# reported as file:line only, so the CI log doesn't repeat it either.

set -euo pipefail

fingerprint="ac1714a0d116e09a0a7a89b76e7c51e8d88ce8fa7678d63ec3c5dec301f32868"

hits=$(git grep -I -n -o -E '[A-Za-z0-9]+' -- . ':(exclude)**/vendor/**' |
  FP="$fingerprint" perl -MDigest::SHA=sha256_hex -ne '
    my ($file, $line, $word) = split /:/, $_, 3;
    chomp $word;
    $word = lc $word;
    $seen{$word} //= sha256_hex($word) eq $ENV{FP};
    print "$file:$line\n" if $seen{$word};
  ' | sort -u)

if [ -n "$hits" ]; then
  echo "$hits"
  echo
  echo "ERROR: operator's production hostname must not appear in tracked source."
  exit 1
fi
echo "OK: no instance hostname references in tracked source."
