# Rolling table library: sources

`library.json` holds the rolling tables Chronicle ships beyond its own starter tables. Every one comes from an openly licensed source, keeps its credit line (shown in the table picker and editor, and carried by "Make a copy"), and is listed below.

Do not edit `library.json` by hand. `node tools/build-roll-table-library.mjs` rebuilds it and this file from the pinned sources; `--check` fails when either is out of date. The build stops when a pinned source changes (its hash no longer matches) or a table's licence is not allowed, so a licence check is a re-run of the build.

## Licences a table may carry

- **CC BY 4.0** (https://creativecommons.org/licenses/by/4.0/): reuse, change and share with credit. Every library table today.

Share-alike and non-commercial licences are not allowed: a campaign's copy must stay free to change. CC0, OGL 1.0a and ORC content could be added, but each needs its own notice handling (the OGL requires its full text and a Section 15 notice) and an entry in the build's allow-list first.

## Sources

### Ironsworn and Ironsworn: Delve

- By Shawn Tomkin, https://www.ironswornrpg.com. Licence: CC BY 4.0.
- Text from the Datasworn transcriptions (https://github.com/rsek/datasworn), pinned: `https://registry.npmjs.org/@datasworn/ironsworn-classic/-/ironsworn-classic-0.0.10.tgz` (sha512-yNoeKcL2qL6I3JH9XfCjMX1u7k83FXnyRzIxQLd32lO9If2Su4Oq6xe4FigQWHsZtkyuYWyj7/B3fMNW24H1xg==) and `https://registry.npmjs.org/@datasworn/ironsworn-classic-delve/-/ironsworn-classic-delve-0.0.10.tgz` (sha512-Q7HScLdcjXHk/0FaleZd5F2RSicMnKfnb8HpulisdK+T7I80VIda330Ff8TWe3o4l/zCIF9+W0n94lM5M0f9WA==). Datasworn records a licence on each oracle; the build keeps only those it marks CC BY 4.0, inheriting a collection's licence for its columns. Both packages also hold CC BY-NC-SA material outside the oracles; none of it is used.
- Notices: "This work is based on Ironsworn (found at www.ironswornrpg.com), created by Shawn Tomkin, and licensed for our use under the Creative Commons Attribution 4.0 International license (creativecommons.org/licenses/by/4.0/)." and "This work is based on Ironsworn: Delve (found at www.ironswornrpg.com), created by Shawn Tomkin, and licensed for our use under the Creative Commons Attribution 4.0 International license (creativecommons.org/licenses/by/4.0/)."
- Adapted: cross-reference links reduced to their words; rows over 120 characters split into a first-sentence name and a brief; d100 ranges kept as weights; the two Ironlander name tables joined into one list; the Settlement Name, Site Name, Site Place and Threat oracles turned into tables that roll on their part lists.
- Left out: oracles tied to Ironsworn's moves and rules (Ask the Oracle, Pay the Price, Endure Harm and Stress, Delve the Depths, Advance a Threat, Challenge Rank, the alternate Reveal a Danger) and the Ironlands Region list, which names the default setting's places.

### System Reference Document 5.1

- By Wizards of the Coast LLC, https://dnd.wizards.com/resources/systems-reference-document. Licence: CC BY 4.0.
- Text from a markdown transcription of the SRD 5.1 (https://github.com/BTMorton/dnd-5e-srd at `bf6ac2ae7e778397ca7326ca991706daf70c13c2`), pinned: `https://raw.githubusercontent.com/BTMorton/dnd-5e-srd/bf6ac2ae7e778397ca7326ca991706daf70c13c2/markdown/03%20beyond1st.md` (sha256 2a3046f27890720736d9cea284e1244d056a2a5c7a67ac141ab1956868d99dfe), `https://raw.githubusercontent.com/BTMorton/dnd-5e-srd/bf6ac2ae7e778397ca7326ca991706daf70c13c2/markdown/09%20running.md` (sha256 e142ba4bf901e2c440069f841a2891355f009271d514b9370eb7c790570f1623), `https://raw.githubusercontent.com/BTMorton/dnd-5e-srd/bf6ac2ae7e778397ca7326ca991706daf70c13c2/markdown/10%20magic%20items.md` (sha256 87d4198ac30e262cbcdb0b9445a89c49e94cd055e3047cd5b2b2d574dc3a6981). Wizards of the Coast released the SRD 5.1 under CC BY 4.0, and that is the licence used here. This copy has not yet been compared word for word against the official CC BY PDF, which this build could not reach.
- Notice: "This work includes material taken from the System Reference Document 5.1 (“SRD 5.1”) by Wizards of the Coast LLC and available at https://dnd.wizards.com/resources/systems-reference-document. The SRD 5.1 is licensed under the Creative Commons Attribution 4.0 International License available at https://creativecommons.org/licenses/by/4.0/legalcode."
- Adapted: a leading "Label:" becomes the line's name; rows over 120 characters split as above; italics dropped.
- Left out: Bag of Beans, Robe of Useful Items and Wand of Wonder, because the transcription merges two rows in each (the build checks every table's ranges cover its die exactly). They can come back from a cleaner copy.

## Tables

| Table | Name | Source | Die | Lines |
|---|---|---|---|---|
| `is-action` | Action | Ironsworn, p. 174 (CC BY 4.0) | d100 | 100 |
| `is-theme` | Theme | Ironsworn, p. 175 (CC BY 4.0) | d100 | 100 |
| `is-character-role` | Character role | Ironsworn, p. 182 (CC BY 4.0) | d100 | 30 |
| `is-character-goal` | Character goal | Ironsworn, p. 182 (CC BY 4.0) | d100 | 33 |
| `is-character-descriptor` | Character descriptor | Ironsworn, p. 183 (CC BY 4.0) | d100 | 100 |
| `is-location` | Location | Ironsworn, p. 176 (CC BY 4.0) | d100 | 51 |
| `is-coastal-location` | Coastal waters location | Ironsworn, p. 176 (CC BY 4.0) | d100 | 17 |
| `is-location-descriptor` | Location descriptor | Ironsworn, p. 177 (CC BY 4.0) | d100 | 50 |
| `is-settlement-trouble` | Settlement trouble | Ironsworn, p. 181 (CC BY 4.0) | d100 | 46 |
| `is-combat-action` | Combat action | Ironsworn, p. 188 (CC BY 4.0) | d100 | 18 |
| `is-mystic-backlash` | Mystic backlash | Ironsworn, p. 189 (CC BY 4.0) | d100 | 25 |
| `is-plot-twist` | Major plot twist | Ironsworn, p. 190 (CC BY 4.0) | d100 | 20 |
| `is-names-ironlander` | Ironlander names | Ironsworn, p. 184 (CC BY 4.0) |  | 200 |
| `is-names-elf` | Elf names | Ironsworn, p. 186 (CC BY 4.0) | d100 | 50 |
| `is-names-giant` | Giant names | Ironsworn (CC BY 4.0) | d100 | 25 |
| `is-names-varou` | Varou names | Ironsworn (CC BY 4.0) | d100 | 25 |
| `is-names-troll` | Troll names | Ironsworn (CC BY 4.0) | d100 | 25 |
| `is-settlement-landscape` | Settlement name: landscape feature | Ironsworn, p. 178 (CC BY 4.0) | d100 | 10 |
| `is-settlement-edifice` | Settlement name: manmade edifice | Ironsworn, p. 178 (CC BY 4.0) | d100 | 10 |
| `is-settlement-creature` | Settlement name: creature | Ironsworn, p. 178 (CC BY 4.0) | d100 | 10 |
| `is-settlement-event` | Settlement name: historical event | Ironsworn, p. 178 (CC BY 4.0) | d100 | 10 |
| `is-settlement-old-word` | Settlement name: Old World word | Ironsworn, p. 178 (CC BY 4.0) | d100 | 10 |
| `is-settlement-aspect` | Settlement name: season or environment | Ironsworn, p. 178 (CC BY 4.0) | d100 | 10 |
| `is-settlement-other` | Settlement name: something else | Ironsworn, p. 178 (CC BY 4.0) | d100 | 10 |
| `is-settlement-name` | Settlement name | Ironsworn, p. 178 (CC BY 4.0) | d100 | 7 |
| `is-settlement-prefix` | Quick settlement name: prefix | Ironsworn (CC BY 4.0) | d100 | 25 |
| `is-settlement-suffix` | Quick settlement name: suffix | Ironsworn (CC BY 4.0) | d100 | 25 |
| `is-settlement-quick-name` | Quick settlement name | Ironsworn (CC BY 4.0) |  | 1 |
| `delve-activity` | Character activity | Ironsworn: Delve, p. 213 (CC BY 4.0) | d100 | 50 |
| `delve-disposition` | Character disposition | Ironsworn: Delve, p. 213 (CC BY 4.0) | d100 | 12 |
| `delve-combat-method` | Combat event: method | Ironsworn: Delve, p. 218 (CC BY 4.0) | d100 | 50 |
| `delve-combat-target` | Combat event: target | Ironsworn: Delve, p. 219 (CC BY 4.0) | d100 | 50 |
| `delve-feature-aspect` | Feature aspect | Ironsworn: Delve, p. 204 (CC BY 4.0) | d100 | 50 |
| `delve-feature-focus` | Feature focus | Ironsworn: Delve, p. 205 (CC BY 4.0) | d100 | 50 |
| `delve-monster-size` | Monstrosity: size | Ironsworn: Delve, p. 214 (CC BY 4.0) | d100 | 6 |
| `delve-monster-form` | Monstrosity: primary form | Ironsworn: Delve, p. 214 (CC BY 4.0) | d100 | 18 |
| `delve-monster-traits` | Monstrosity: characteristics | Ironsworn: Delve, p. 215 (CC BY 4.0) | d100 | 28 |
| `delve-monster-abilities` | Monstrosity: abilities | Ironsworn: Delve, p. 216 (CC BY 4.0) | d100 | 40 |
| `delve-opportunity` | Find an opportunity | Ironsworn: Delve, p. 30 (CC BY 4.0) | d100 | 10 |
| `delve-danger` | Reveal a danger | Ironsworn: Delve, p. 34 (CC BY 4.0) | d100 | 12 |
| `delve-site-theme` | Site theme | Ironsworn: Delve, p. 212 (CC BY 4.0) | d100 | 8 |
| `delve-site-domain` | Site domain | Ironsworn: Delve, p. 212 (CC BY 4.0) | d100 | 12 |
| `delve-trap-event` | Trap event | Ironsworn: Delve, p. 217 (CC BY 4.0) | d100 | 25 |
| `delve-trap-component` | Trap component | Ironsworn: Delve, p. 217 (CC BY 4.0) | d100 | 25 |
| `delve-site-description` | Site name: description | Ironsworn: Delve, p. 207 (CC BY 4.0) | d100 | 50 |
| `delve-site-detail` | Site name: detail | Ironsworn: Delve, p. 208 (CC BY 4.0) | d100 | 50 |
| `delve-site-namesake` | Site name: namesake | Ironsworn: Delve, p. 209 (CC BY 4.0) | d100 | 50 |
| `delve-place-barrow` | Site name: barrow place | Ironsworn: Delve, p. 210 (CC BY 4.0) | d100 | 6 |
| `delve-place-cavern` | Site name: cavern place | Ironsworn: Delve, p. 210 (CC BY 4.0) | d100 | 10 |
| `delve-place-frozen-cavern` | Site name: frozen cavern place | Ironsworn: Delve, p. 210 (CC BY 4.0) | d100 | 10 |
| `delve-place-icereach` | Site name: icereach place | Ironsworn: Delve, p. 210 (CC BY 4.0) | d100 | 6 |
| `delve-place-mine` | Site name: mine place | Ironsworn: Delve, p. 210 (CC BY 4.0) | d100 | 6 |
| `delve-place-pass` | Site name: pass place | Ironsworn: Delve, p. 210 (CC BY 4.0) | d100 | 10 |
| `delve-place-ruin` | Site name: ruin place | Ironsworn: Delve, p. 211 (CC BY 4.0) | d100 | 10 |
| `delve-place-sea-cave` | Site name: sea cave place | Ironsworn: Delve, p. 211 (CC BY 4.0) | d100 | 6 |
| `delve-place-shadowfen` | Site name: shadowfen place | Ironsworn: Delve, p. 211 (CC BY 4.0) | d100 | 10 |
| `delve-place-stronghold` | Site name: stronghold place | Ironsworn: Delve, p. 211 (CC BY 4.0) | d100 | 10 |
| `delve-place-tanglewood` | Site name: tanglewood place | Ironsworn: Delve, p. 211 (CC BY 4.0) | d100 | 8 |
| `delve-place-underkeep` | Site name: underkeep place | Ironsworn: Delve, p. 211 (CC BY 4.0) | d100 | 10 |
| `delve-site-place` | Site name: place | Ironsworn: Delve, p. 210 (CC BY 4.0) | d100 | 12 |
| `delve-site-name` | Site name | Ironsworn: Delve (CC BY 4.0) | d100 | 7 |
| `delve-threat-burgeoning-conflict` | Threat: burgeoning conflict | Ironsworn: Delve, p. 221 (CC BY 4.0) | d100 | 10 |
| `delve-threat-cursed-site` | Threat: cursed site | Ironsworn: Delve, p. 221 (CC BY 4.0) | d100 | 10 |
| `delve-threat-environmental-calamity` | Threat: environmental calamity | Ironsworn: Delve, p. 221 (CC BY 4.0) | d100 | 10 |
| `delve-threat-malignant-plague` | Threat: malignant plague | Ironsworn: Delve, p. 222 (CC BY 4.0) | d100 | 10 |
| `delve-threat-rampaging-creature` | Threat: rampaging creature | Ironsworn: Delve, p. 222 (CC BY 4.0) | d100 | 10 |
| `delve-threat-ravaging-horde` | Threat: ravaging horde | Ironsworn: Delve, p. 222 (CC BY 4.0) | d100 | 10 |
| `delve-threat-scheming-leader` | Threat: scheming leader | Ironsworn: Delve, p. 223 (CC BY 4.0) | d100 | 10 |
| `delve-threat-power-hungry-mystic` | Threat: power-hungry mystic | Ironsworn: Delve, p. 223 (CC BY 4.0) | d100 | 10 |
| `delve-threat-zealous-cult` | Threat: zealous cult | Ironsworn: Delve, p. 223 (CC BY 4.0) | d100 | 10 |
| `delve-threat` | Threat | Ironsworn: Delve, p. 220 (CC BY 4.0) | d100 | 10 |
| `srd-madness-short` | Short-term madness | System Reference Document 5.1 (CC BY 4.0) | d100 | 10 |
| `srd-madness-long` | Long-term madness | System Reference Document 5.1 (CC BY 4.0) | d100 | 12 |
| `srd-madness-indefinite` | Indefinite madness | System Reference Document 5.1 (CC BY 4.0) | d100 | 12 |
| `srd-acolyte-trait` | Acolyte: personality trait | System Reference Document 5.1 (CC BY 4.0) | d8 | 8 |
| `srd-acolyte-ideal` | Acolyte: ideal | System Reference Document 5.1 (CC BY 4.0) | d6 | 6 |
| `srd-acolyte-bond` | Acolyte: bond | System Reference Document 5.1 (CC BY 4.0) | d6 | 6 |
| `srd-acolyte-flaw` | Acolyte: flaw | System Reference Document 5.1 (CC BY 4.0) | d6 | 6 |
| `srd-efreeti-bottle` | Efreeti bottle | System Reference Document 5.1 (CC BY 4.0) | d100 | 3 |
| `srd-sentient-communication` | Sentient item: communication | System Reference Document 5.1 (CC BY 4.0) | d100 | 3 |
| `srd-sentient-senses` | Sentient item: senses | System Reference Document 5.1 (CC BY 4.0) | d4 | 4 |
| `srd-sentient-alignment` | Sentient item: alignment | System Reference Document 5.1 (CC BY 4.0) | d100 | 9 |
| `srd-sentient-purpose` | Sentient item: special purpose | System Reference Document 5.1 (CC BY 4.0) | d10 | 10 |
