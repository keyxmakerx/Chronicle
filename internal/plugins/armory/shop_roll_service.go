// shop_roll_service.go lets a player haggle over a shop's good or look closer
// at it, with one of their characters. The server throws the dice; the page
// only shows them landing. A haggle changes the price the buy route charges
// that character for that good on that shelf day, and a look can uncover the
// owner's hidden note, which stays uncovered for that character.
//
// The rules that must not drift: one try per good, character, action and
// shelf day (the table's unique key is the arbiter, so two fast taps cannot
// both roll); the hidden note and extra are read only through a reveal and
// never reach the layout or the roll log.
package armory

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	haggledTwiceMessage   = "You've already haggled over this today."
	lookedTwiceMessage    = "You've already looked closer at this today."
	haggleOffMessage      = "Haggling is not on at this shop."
	lookOffMessage        = "Looking closer is not on at this shop."
	haggleNoPriceMessage  = "There is no price to haggle over."
	chooseCharacterMsg    = "Choose a character."
	nothingMoreToSeeLabel = "Nothing more to see."
)

// ShopRollService is the counter rolls and the owner's hidden notes.
type ShopRollService interface {
	// Rolls describes the test and the caller's character's state at a shop.
	// With no character only the Owner may ask, for the settings view.
	Rolls(ctx context.Context, campaignID, shopEntityID string, a Actor, characterID string) (*RollsView, error)
	// Roll throws one haggle or look for a character and applies the result.
	Roll(ctx context.Context, campaignID, shopEntityID string, a Actor, in RollInput) (*RollResult, error)
	// Secrets lists the shop's hidden notes by relation id. Owner only.
	Secrets(ctx context.Context, campaignID, shopEntityID string, a Actor) (map[string]GoodSecret, error)
	// SaveSecret sets one good's note and extra; both empty deletes them.
	// Owner only.
	SaveSecret(ctx context.Context, campaignID, shopEntityID string, relationID int, a Actor, in GoodSecret) (*GoodSecret, error)
}

// ShopRollDeps are what the service needs.
type ShopRollDeps struct {
	Repo     ShopRollRepository
	Rooms    ShopRoomRepository
	Stash    *stashService
	Shops    ShopEntityChecker
	Rels     RelationFinder
	Listings ShopListingLister
	Today    ShopTodayService
	Rules    ShopTestRulesReader
	Sheets   SheetStatReader
	// Log is optional; without it rolls are made but not logged.
	Log ShopRollLog
}

type shopRollService struct {
	ShopRollDeps
	roll func(sides int) int
}

// NewShopRollService creates the service. Dice come from the system's secure
// random source.
func NewShopRollService(d ShopRollDeps) ShopRollService {
	return &shopRollService{ShopRollDeps: d, roll: cryptoDie}
}

// cryptoDie is one die of n sides. A failed read of the random source rolls a
// 1 rather than failing the roll.
func cryptoDie(n int) int {
	if n < 2 {
		return 1
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 1
	}
	return int(v.Int64()) + 1
}

// settings reads the owner's counter settings from the saved layout. A shop
// with no layout, or none for rolls, has both rolls off.
func (s *shopRollService) settings(ctx context.Context, campaignID, shopEntityID string) (ShopRoomRolls, error) {
	raw, err := s.Rooms.Get(ctx, campaignID, shopEntityID)
	if err != nil {
		return ShopRoomRolls{}, err
	}
	off := ShopRoomRolls{Stubborn: defaultRollStubborn, Great: defaultRollGreat, Middling: defaultRollMiddling, Poor: defaultRollPoor}
	if len(raw) == 0 {
		return off, nil
	}
	var l struct {
		Rolls *ShopRoomRolls `json:"rolls"`
	}
	if err := json.Unmarshal(raw, &l); err != nil || l.Rolls == nil {
		return off, nil
	}
	return *l.Rolls, nil
}

// statPick is the stat an action adds, decided from the owner's choice and the
// system's default.
type statPick struct {
	stat  ShopTestStat
	named bool // false when no stat is configured at all
}

func pickStat(rules ShopTestRules, owner, action string) statPick {
	key := strings.TrimSpace(owner)
	if key == "" {
		key = rules.Haggle
		if action == RollLook {
			key = rules.Look
		}
	}
	if key == "" {
		return statPick{}
	}
	for _, st := range rules.Stats {
		if st.Key == key {
			return statPick{stat: st, named: true}
		}
	}
	// The owner may name a sheet field the system does not list; it is read
	// as a plain number under its own key.
	return statPick{stat: ShopTestStat{Key: key, Label: key}, named: true}
}

// statModifier is what the stat adds to the roll: the ability-score modifier
// for a score, the value itself otherwise.
func statModifier(st ShopTestStat, value int) int {
	if !st.Score {
		return value
	}
	x := value - 10
	if x < 0 {
		return -((-x + 1) / 2)
	}
	return x / 2
}

// stubbornMod is what the keeper's stubbornness adds under style tiers: an
// easy keeper helps, a hard one hinders.
func stubbornMod(rules ShopTestRules, stubborn string) int {
	if rules.Style != testStyleTiers {
		return 0
	}
	switch stubborn {
	case "easy":
		return rules.Edge
	case "hard":
		return rules.Bane
	}
	return 0
}

func dcFor(rules ShopTestRules, stubborn string) int {
	switch stubborn {
	case "easy":
		return rules.DCEasy
	case "hard":
		return rules.DCHard
	}
	return rules.DCUsual
}

// tierOf bands a total into 1, 2 or 3. Under tiers the totals up to the first
// cut-off are tier 1 and up to the second tier 2. Under dc, meeting the DC is
// tier 2 and beating it by 5 is tier 3.
func tierOf(rules ShopTestRules, stubborn string, total int) int {
	if rules.Style == testStyleTiers {
		lo, hi := 11, 16
		if len(rules.Tiers) == 2 {
			lo, hi = rules.Tiers[0], rules.Tiers[1]
		}
		switch {
		case total <= lo:
			return 1
		case total <= hi:
			return 2
		}
		return 3
	}
	dc := dcFor(rules, stubborn)
	switch {
	case total >= dc+5:
		return 3
	case total >= dc:
		return 2
	}
	return 1
}

// againstLabel is what the total is held against, in words.
func againstLabel(rules ShopTestRules, stubborn string) string {
	if rules.Style == testStyleTiers {
		return strings.ToUpper(stubborn[:1]) + stubborn[1:]
	}
	return fmt.Sprintf("DC %d", dcFor(rules, stubborn))
}

// haggleChange is the price change in percent a haggle tier wins: a discount
// for the top two tiers, a markup for a failure.
func haggleChange(r ShopRoomRolls, tier int) int {
	switch tier {
	case 3:
		return -r.Great
	case 2:
		return -r.Middling
	}
	return r.Poor
}

func haggleOutcome(pct int) string {
	switch {
	case pct < 0:
		return fmt.Sprintf("%d%% off", -pct)
	case pct > 0:
		return fmt.Sprintf("Price up %d%%", pct)
	}
	return "Price unchanged"
}

// normalStubborn guards against a hand-edited layout.
func normalStubborn(s string) string {
	if shopRollStubborn[s] {
		return s
	}
	return defaultRollStubborn
}

func (s *shopRollService) statChoices(rules ShopTestRules) []RollStatChoice {
	out := make([]RollStatChoice, 0, len(rules.Stats))
	for _, st := range rules.Stats {
		out = append(out, RollStatChoice{Key: st.Key, Label: st.Label})
	}
	return out
}

// statView describes what an action adds. With a character it reads the sheet.
func (s *shopRollService) statView(ctx context.Context, campaignID, characterID string, p statPick) (RollStatView, error) {
	v := RollStatView{Stat: p.stat.Key, Label: p.stat.Label}
	if characterID == "" {
		return v, nil
	}
	value, missing := 0, false
	mod := 0
	if p.named {
		n, ok, err := s.Sheets.Stat(ctx, campaignID, characterID, p.stat.Key)
		if err != nil {
			return v, apperror.NewInternal(err)
		}
		if ok {
			value = n
			mod = statModifier(p.stat, n)
		} else {
			missing = true
		}
	}
	v.Value, v.Missing, v.Mod = &value, &missing, &mod
	return v, nil
}

func (s *shopRollService) Rolls(ctx context.Context, campaignID, shopEntityID string, a Actor, characterID string) (*RollsView, error) {
	if err := requireViewableShop(ctx, s.Shops, s.Stash.Visibility, campaignID, shopEntityID, a.Role, a.UserID); err != nil {
		return nil, err
	}
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		// Only the Owner may look without a character: it is the settings view.
		if !a.IsOwner() {
			return nil, apperror.NewBadRequest(chooseCharacterMsg)
		}
	} else if _, err := s.authorizedCharacter(ctx, campaignID, a, characterID); err != nil {
		return nil, err
	}
	set, err := s.settings(ctx, campaignID, shopEntityID)
	if err != nil {
		return nil, err
	}
	rules := s.Rules.ShopTestRules(ctx, campaignID)
	stubborn := normalStubborn(set.Stubborn)
	hv, err := s.statView(ctx, campaignID, characterID, pickStat(rules, set.HaggleStat, RollHaggle))
	if err != nil {
		return nil, err
	}
	lv, err := s.statView(ctx, campaignID, characterID, pickStat(rules, set.LookStat, RollLook))
	if err != nil {
		return nil, err
	}
	view := &RollsView{
		Haggle: set.Haggle, Look: set.Look,
		Test: RollTestView{Dice: rules.Dice, Style: rules.Style, Haggle: hv, Look: lv,
			Against: againstLabel(rules, stubborn), StubbornMod: stubbornMod(rules, stubborn)},
		Stats:    s.statChoices(rules),
		Defaults: RollDefaults{Haggle: rules.Haggle, Look: rules.Look},
		Goods:    map[string]RollGoodView{},
	}
	if characterID == "" {
		return view, nil
	}
	if err := s.fillGoods(ctx, campaignID, shopEntityID, a, characterID, view); err != nil {
		return nil, err
	}
	return view, nil
}

// fillGoods adds, for each good the caller may still see, today's haggle and
// the best look the character has made on any day.
func (s *shopRollService) fillGoods(ctx context.Context, campaignID, shopEntityID string, a Actor, characterID string, view *RollsView) error {
	rows, err := s.Repo.CharacterRolls(ctx, campaignID, shopEntityID, characterID)
	if err != nil || len(rows) == 0 {
		return err
	}
	day, err := s.Today.ShelfDayIndex(ctx, campaignID)
	if err != nil {
		return err
	}
	listings, err := s.Listings.ListSells(ctx, campaignID, shopEntityID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	live := map[int]ShopListing{}
	targets := make([]string, 0, len(listings))
	for _, l := range listings {
		if l.DmOnly && !a.SeesDmOnly() {
			continue
		}
		live[l.RelationID] = l
		targets = append(targets, l.TargetEntityID)
	}
	if !a.IsOwner() {
		if s.Stash.Visibility == nil {
			return nil
		}
		seen, err := s.Stash.Visibility.FilterViewableEntityIDs(ctx, campaignID, targets, a.Role, a.UserID)
		if err != nil {
			return apperror.NewInternal(err)
		}
		for id, l := range live {
			if !seen[l.TargetEntityID] {
				delete(live, id)
			}
		}
	}
	secrets, err := s.Repo.Secrets(ctx, campaignID, shopEntityID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		l, ok := live[r.RelationID]
		if !ok {
			continue
		}
		key := fmt.Sprint(r.RelationID)
		g := view.Goods[key]
		switch r.Action {
		case RollHaggle:
			if r.ShelfDay != day {
				continue
			}
			listed, err := unitPriceCents(parseShopMeta(l.Metadata).Price)
			if err != nil {
				continue
			}
			g.Haggle = &GoodHaggleView{Tier: r.Tier, Pct: r.Pct, Price: centsToFloat(haggledUnit(listed, r.Pct)), Was: centsToFloat(listed)}
		case RollLook:
			today := r.ShelfDay == day || (g.Look != nil && g.Look.Today)
			// Rows come oldest first: a later look of the same tier wins, a
			// lower tier never takes back what a better one uncovered.
			if g.Look == nil || r.Tier >= g.Look.Tier {
				g.Look = &GoodLookView{Tier: r.Tier}
			}
			g.Look.Today = today
		}
		view.Goods[key] = g
	}
	for key, g := range view.Goods {
		if g.Look != nil {
			var id int
			_, _ = fmt.Sscan(key, &id)
			g.Look.Note, g.Look.Extra = revealed(g.Look.Tier, secrets[id])
			view.Goods[key] = g
		}
	}
	return nil
}

// revealed is what a look of this tier uncovers of a secret: the note from
// tier 2, the extra only at tier 3.
func revealed(tier int, sec GoodSecret) (note, extra string) {
	if tier >= 2 {
		note = sec.Note
	}
	if tier >= 3 {
		extra = sec.Extra
	}
	return note, extra
}

// authorizedCharacter loads the character and applies the buyers rule.
func (s *shopRollService) authorizedCharacter(ctx context.Context, campaignID string, a Actor, characterID string) (*EntityRef, error) {
	ch, err := s.Stash.loadCharacter(ctx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	if err := authorizeBuyerFor(ctx, s.Stash, campaignID, a, ch.ID); err != nil {
		return nil, err
	}
	return ch, nil
}

func (s *shopRollService) Roll(ctx context.Context, campaignID, shopEntityID string, a Actor, in RollInput) (*RollResult, error) {
	if in.Action != RollHaggle && in.Action != RollLook {
		return nil, apperror.NewBadRequest("Choose haggle or look.")
	}
	characterID := strings.TrimSpace(in.CharacterID)
	if characterID == "" {
		return nil, apperror.NewBadRequest(chooseCharacterMsg)
	}
	if in.RelationID <= 0 {
		return nil, apperror.NewBadRequest("Choose an item.")
	}
	if err := requireViewableShop(ctx, s.Shops, s.Stash.Visibility, campaignID, shopEntityID, a.Role, a.UserID); err != nil {
		return nil, err
	}
	ch, err := s.authorizedCharacter(ctx, campaignID, a, characterID)
	if err != nil {
		return nil, err
	}
	set, err := s.settings(ctx, campaignID, shopEntityID)
	if err != nil {
		return nil, err
	}
	switch {
	case in.Action == RollHaggle && !set.Haggle:
		return nil, apperror.NewForbidden(haggleOffMessage)
	case in.Action == RollLook && !set.Look:
		return nil, apperror.NewForbidden(lookOffMessage)
	}

	rel, err := findShopListing(ctx, s.Rels, campaignID, shopEntityID, a, int(in.RelationID))
	if err != nil {
		return nil, err
	}
	if !a.IsOwner() {
		if s.Stash.Visibility == nil {
			return nil, notFound("shop item")
		}
		seen, err := s.Stash.Visibility.FilterViewableEntityIDs(ctx, campaignID, []string{rel.TargetEntityID}, a.Role, a.UserID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if !seen[rel.TargetEntityID] {
			return nil, notFound("shop item")
		}
	}
	meta := parseShopMeta(rel.Metadata)
	on, err := s.Today.OnShelf(ctx, campaignID, shopEntityID, int(in.RelationID), meta)
	if err != nil {
		return nil, err
	}
	if !on {
		return nil, apperror.NewBadRequest(notOnShelfMessage)
	}
	var listed Cents
	if in.Action == RollHaggle {
		if listed, err = unitPriceCents(meta.Price); err != nil {
			return nil, apperror.NewBadRequest(haggleNoPriceMessage)
		}
	}
	day, err := s.Today.ShelfDayIndex(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	// Everything that can fail is read before the dice are thrown and the row
	// is written, so a failed read never spends the day's one try.
	rules := s.Rules.ShopTestRules(ctx, campaignID)
	stubborn := normalStubborn(set.Stubborn)
	owner := set.HaggleStat
	if in.Action == RollLook {
		owner = set.LookStat
	}
	pick := pickStat(rules, owner, in.Action)
	sv, err := s.statView(ctx, campaignID, ch.ID, pick)
	if err != nil {
		return nil, err
	}
	statMod := 0
	if sv.Mod != nil {
		statMod = *sv.Mod
	}
	statMissing := sv.Missing != nil && *sv.Missing

	count, sides, ok := parseDice(rules.Dice)
	if !ok {
		count, sides = 1, 20
	}
	dice := make([]RollDie, count)
	sum := 0
	for i := range dice {
		dice[i] = RollDie{Sides: sides, Value: s.roll(sides)}
		sum += dice[i].Value
	}
	modifier := statMod + stubbornMod(rules, stubborn)
	total := sum + modifier
	tier := tierOf(rules, stubborn, total)

	row := ShopRoll{CampaignID: campaignID, ShopEntityID: shopEntityID, RelationID: int(in.RelationID),
		CharacterID: ch.ID, Action: in.Action, ShelfDay: day, Tier: tier, Total: total}
	if in.Action == RollHaggle {
		row.Pct = haggleChange(set, tier)
	}
	if _, err := s.Repo.InsertRoll(ctx, row); err != nil {
		if errors.Is(err, errAlreadyTried) {
			if in.Action == RollHaggle {
				return nil, apperror.NewConflict(haggledTwiceMessage)
			}
			return nil, apperror.NewConflict(lookedTwiceMessage)
		}
		return nil, asAppError(err)
	}

	label := pick.stat.Label + " test"
	if !pick.named {
		label = "Haggle test"
		if in.Action == RollLook {
			label = "Look closer test"
		}
	}
	res := &RollResult{Action: in.Action, Dice: dice, Modifier: modifier, Total: total, Tier: tier, Label: label, StatMissing: statMissing}
	logLabel := "Haggle: "
	switch in.Action {
	case RollHaggle:
		res.Outcome = haggleOutcome(row.Pct)
		res.Haggle = &RollHaggleResult{Pct: row.Pct, Price: centsToFloat(haggledUnit(listed, row.Pct)), Was: centsToFloat(listed), Currency: normCurrency(meta.Currency)}
	case RollLook:
		logLabel = "Look closer: "
		res.Look = &RollLookResult{}
		if tier >= 2 {
			res.Outcome = "Noticed something"
			secrets, err := s.Repo.Secrets(ctx, campaignID, shopEntityID)
			if err != nil {
				// The reveal is stored by tier, so the next read of the
				// counter shows it even though this answer could not.
				return nil, err
			}
			res.Look.Note, res.Look.Extra = revealed(tier, secrets[int(in.RelationID)])
		} else {
			res.Outcome = nothingMoreToSeeLabel
		}
	}
	res.RollID = s.logRoll(ctx, campaignID, shopEntityID, rel.TargetEntityID, a, ch, logLabel, rules.Dice, dice, modifier, total, res)
	return res, nil
}

// logRoll adds the roll to the public log and returns the log's id. The log
// holds the dice and the outcome only, never a note or extra: its outcome for a
// look is the same whether or not the owner wrote anything.
func (s *shopRollService) logRoll(ctx context.Context, campaignID, shopID, itemID string, a Actor, ch *EntityRef, label, formula string, dice []RollDie, modifier, total int, res *RollResult) string {
	if s.Log == nil {
		return ""
	}
	good, shop := "an item", "the shop"
	if e, err := s.Stash.Directory.GetEntity(ctx, campaignID, itemID); err == nil && e != nil && e.Name != "" {
		good = e.Name
	}
	if e, err := s.Stash.Directory.GetEntity(ctx, campaignID, shopID); err == nil && e != nil && e.Name != "" {
		shop = e.Name
	}
	outcome := res.Outcome
	if res.Action == RollLook && res.Tier < 2 {
		outcome = "Nothing more"
	}
	return s.Log.Rolled(context.WithoutCancel(ctx), campaignID, ShopRollLogEntry{
		RollerID: a.UserID, RollerName: ch.Name, Label: label + good, Formula: formula,
		Dice: dice, Modifier: modifier, Total: total, Outcome: outcome, From: shop,
	})
}

func (s *shopRollService) Secrets(ctx context.Context, campaignID, shopEntityID string, a Actor) (map[string]GoodSecret, error) {
	if !a.IsOwner() {
		return nil, apperror.NewForbidden("Only the owner can read a shop's notes.")
	}
	if err := requireShopInCampaign(ctx, s.Shops, campaignID, shopEntityID); err != nil {
		return nil, err
	}
	rows, err := s.Repo.Secrets(ctx, campaignID, shopEntityID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]GoodSecret, len(rows))
	for id, sec := range rows {
		out[fmt.Sprint(id)] = sec
	}
	return out, nil
}

func (s *shopRollService) SaveSecret(ctx context.Context, campaignID, shopEntityID string, relationID int, a Actor, in GoodSecret) (*GoodSecret, error) {
	if !a.IsOwner() {
		return nil, apperror.NewForbidden("Only the owner can write a shop's notes.")
	}
	if err := requireShopInCampaign(ctx, s.Shops, campaignID, shopEntityID); err != nil {
		return nil, err
	}
	note, err := cleanSecretText(in.Note, "note")
	if err != nil {
		return nil, err
	}
	extra, err := cleanSecretText(in.Extra, "extra")
	if err != nil {
		return nil, err
	}
	// A note may only sit on a real listing of this shop.
	if _, err := findShopListing(ctx, s.Rels, campaignID, shopEntityID, a, relationID); err != nil {
		return nil, err
	}
	out := GoodSecret{Note: note, Extra: extra}
	if err := s.Repo.SaveSecret(ctx, campaignID, shopEntityID, relationID, out); err != nil {
		return nil, err
	}
	return &out, nil
}

// cleanSecretText trims plain text and refuses what is too long or holds
// control characters. Markup is not stripped: every place that shows it
// writes it as text.
func cleanSecretText(s, name string) (string, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	if utf8.RuneCountInString(s) > maxGoodSecretChars {
		return "", apperror.NewBadRequest(fmt.Sprintf("The %s can be at most %d characters.", name, maxGoodSecretChars))
	}
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f {
			return "", apperror.NewBadRequest(fmt.Sprintf("The %s must be plain text.", name))
		}
	}
	return s, nil
}
