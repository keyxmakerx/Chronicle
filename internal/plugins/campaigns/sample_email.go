package campaigns

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	mailbody "github.com/keyxmakerx/chronicle/internal/email"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// TestMailer sends the Customize page's "Send me a test" email. Subset of
// smtp.MailService, like InviteMailer.
type TestMailer interface {
	SendHTMLMail(ctx context.Context, to []string, subject, plainBody, htmlBody string) error
	IsConfigured(ctx context.Context) bool
}

// SetTestMailer wires the mail sender for "Send me a test".
func (h *Handler) SetTestMailer(m TestMailer) {
	h.testMailer = m
}

// EmailLayoutOf is the layout the owner picked for the campaign's emails,
// Note when unset or unknown.
func EmailLayoutOf(c *Campaign) mailbody.Layout {
	if c == nil {
		return mailbody.DefaultLayout
	}
	if st := c.ParseSettings(); st.Appearance != nil {
		return mailbody.ParseLayout(st.Appearance.EmailLayout)
	}
	return mailbody.DefaultLayout
}

// emailTestInput is the draft the test is drawn from: the layout and colour
// picked on the page, saved or not.
type emailTestInput struct {
	Layout string `json:"layout"`
	Accent string `json:"accent"`
}

// SendTestEmailAPI handles POST /campaigns/:id/appearance/email-test. It
// sends the owner a sample game night invite in the picked layout, to the
// address on their own account only, so the route can't mail anyone else.
func (h *Handler) SendTestEmailAPI(c *echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if cc.MemberRole < RoleOwner {
		return apperror.NewForbidden("only campaign owners can send a test email")
	}
	var in emailTestInput
	if err := c.Bind(&in); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	layout, err := oneOf("email layout", strings.TrimSpace(in.Layout), AppearanceEmailLayouts)
	if err != nil {
		return err
	}
	sess := auth.GetSession(c)
	if sess == nil || strings.TrimSpace(sess.Email) == "" {
		return apperror.NewBadRequest("Your account has no email address to send the test to.")
	}
	ctx := c.Request().Context()
	if h.testMailer == nil || !h.testMailer.IsConfigured(ctx) {
		return apperror.NewBadRequest("This site can't send email yet. A site admin sets it up under Admin › Email.")
	}
	// The page context knows the game's word for its game master.
	term := layouts.DefaultGMTerm
	if middleware.LayoutInjector != nil {
		term = layouts.GMTerm(middleware.LayoutInjector(c, ctx))
	}
	plain, html, err := mailbody.Render(mailbody.ParseLayout(layout), h.sampleEmail(cc.Campaign, sess.Name, term, in.Accent, time.Now()))
	if err != nil {
		return apperror.NewInternal(err)
	}
	subject := fmt.Sprintf("Test: how %s's emails look", cc.Campaign.Name)
	if err := h.testMailer.SendHTMLMail(ctx, []string{sess.Email}, subject, plain, html); err != nil {
		return apperror.NewBadRequest("The mail server didn't take the test. Check Admin › Email, then try again.")
	}
	return c.JSON(http.StatusOK, map[string]string{"to": sess.Email})
}

// sampleEmail is a game night invite like the ones players get, on next
// Friday at 7pm, from the owner. Its answers open the Customize page again,
// so pressing one in the test records nothing.
func (h *Handler) sampleEmail(c *Campaign, ownerName, gmTerm, accent string, now time.Time) mailbody.Message {
	st := c.ParseSettings()
	if accent == "" {
		accent = st.AccentColor
	}
	friday := now.AddDate(0, 0, (int(time.Friday)-int(now.Weekday())+7)%7)
	if friday.YearDay() == now.YearDay() && friday.Year() == now.Year() {
		friday = friday.AddDate(0, 0, 7)
	}
	back := h.baseURL + "/campaigns/" + c.ID + "/customize/look#email"
	m := mailbody.Message{
		Campaign: c.Name,
		Accent:   accent,
		Title:    "A sample game night",
		Lines: []string{
			friday.Format("Monday 2 January") + ", 7pm. Can you make it?",
			"This is a test of how " + c.Name + "'s emails look. The answers below only open Customize.",
		},
		Answers: []mailbody.Answer{{Label: "Going", URL: back}, {Label: "Maybe", URL: back}, {Label: "Can't make it", URL: back}},
		Links:   []mailbody.Answer{{Label: "Suggest another time", URL: back}},
		Small:   "In a real invite, these links expire in 7 days.",
		Why:     "You get this because you pressed Send me a test in Customize.",
		Stub:    &mailbody.Stub{Weekday: strings.ToUpper(friday.Format("Mon")), Day: friday.Format("2"), Month: strings.ToUpper(friday.Format("Jan")), Time: "7pm"},
	}
	if name := strings.TrimSpace(ownerName); name != "" {
		m.Sender = &mailbody.Sender{Name: name, Role: "your " + gmTerm}
	}
	return m
}
