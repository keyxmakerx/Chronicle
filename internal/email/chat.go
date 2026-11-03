package email

// chatHTML is the Chat layout: the email reads as a message from the person
// who sent it, with a round initial in the campaign colour, a speech bubble
// holding the words and a small card with the title, and the answers as
// reply chips on the right, the first one filled. Without a sender it reads
// as from the campaign. On a phone the bubble fills the width and the chips
// become full-width rows.
const chatHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light"><meta name="supported-color-schemes" content="light">
<title>{{.Title}}</title>
<style>
@media (max-width:600px){
.c-pane{padding:18px 12px !important}
.c-bub{margin-left:0 !important}
.c-ans{text-align:left !important}
.c-ans a{display:block !important;margin:0 0 8px 0 !important;text-align:center !important}
}
</style></head>
<body style="margin:0;padding:0;background:#f6f7f9">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#f6f7f9"><tr>
<td class="c-pane" align="center" style="padding:36px 48px">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:480px"><tr>
<td style="font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:16px;line-height:1.5;color:#2b2f36">
<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:0 0 10px"><tr>
<td width="34" valign="middle" style="width:34px;height:34px;border-radius:50%;background:{{.Accent}};color:#ffffff;font-size:15px;font-weight:700;text-align:center;line-height:34px">{{.Initial}}</td>
<td valign="middle" style="padding-left:10px;font-size:13px;color:#6b7280">
{{- if .Sender}}<b style="color:#111111;font-size:14px">{{.Sender.Name}}{{if .Sender.Role}}, {{.Sender.Role}}{{end}}</b>{{if .Campaign}} in {{.Campaign}}{{end}}
{{- else}}<b style="color:#111111;font-size:14px">{{.Campaign}}</b>{{end}}
</td></tr></table>
<div class="c-bub" style="margin:0 0 10px 44px;padding:12px 16px;background:#ffffff;border:1px solid #e3e5ea;border-radius:4px 16px 16px 16px">
{{- range .Lines}}
<p style="margin:0 0 8px">{{.}}</p>
{{- end}}
<p style="margin:8px 0 0;padding:10px 12px;border-radius:10px;background:#f3f4f6;border-left:3px solid {{.Accent}};font-size:15px;font-weight:600;color:#111111">{{.Title}}</p>
{{- range .Groups}}
<p style="margin:12px 0 0;font-weight:600;color:#111111">{{.Title}}{{if .Detail}} <span style="font-weight:400;color:#6b7280">{{.Detail}}</span>{{end}}</p>
<p class="c-ans" style="margin:6px 0 0">{{range $i, $a := .Answers}}<a href="{{$a.URL}}" style="display:inline-block;margin:0 6px 6px 0;padding:0 16px;line-height:38px;border-radius:999px;font-size:14px;font-weight:600;text-decoration:none;{{if eq $i 0}}background:{{$.Accent}};color:#ffffff{{else}}background:#ffffff;color:{{$.Accent}};border:1px solid #c7c9d1{{end}}">{{$a.Label}}</a> {{end}}</p>
{{- end}}
</div>
{{- if .Answers}}
<p class="c-ans" style="margin:0 0 10px;text-align:right">{{range $i, $a := .Answers}}<a href="{{$a.URL}}" style="display:inline-block;margin:0 0 8px 8px;padding:0 18px;line-height:42px;border-radius:999px;font-size:15px;font-weight:600;text-decoration:none;{{if eq $i 0}}background:{{$.Accent}};color:#ffffff{{else}}background:#ffffff;color:{{$.Accent}};border:1px solid #c7c9d1{{end}}">{{$a.Label}}</a> {{end}}</p>
{{- end}}
{{- if .Links}}
<p style="margin:0 0 10px;font-size:14px;text-align:right">{{range .Links}}<a href="{{.URL}}" style="color:{{$.Accent}};text-decoration:underline;margin-left:16px">{{.Label}}</a> {{end}}</p>
{{- end}}
{{- if .Small}}
<p style="margin:0;font-size:13px;color:#8a8f98;text-align:center">{{.Small}}</p>
{{- end}}
{{- if .Why}}
<p style="margin:0;padding-top:8px;font-size:12px;line-height:1.5;color:#a0a4ab;text-align:center">{{.Why}}{{if .PrefsURL}} <a href="{{.PrefsURL}}" style="color:#a0a4ab;text-decoration:underline">Change what reaches you</a>{{end}}</p>
{{- end}}
</td></tr></table>
</td></tr></table>
</body></html>
`
