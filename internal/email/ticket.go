package email

// ticketHTML is the Ticket layout: a white ticket on a grey ground, the date
// on a stub in the campaign colour (in a typewriter face) beside the name,
// and square bordered buttons with the first one filled. An email with no
// single date, such as a date poll, keeps the ticket without its stub. On a
// phone the stub sits across the top and the buttons become full-width rows.
const ticketHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light"><meta name="supported-color-schemes" content="light">
<title>{{.Title}}</title>
<style>
@media (max-width:600px){
.t-pane{padding:16px 12px !important}
.t-stub,.t-in{display:block !important;width:auto !important}
.t-stub{border-right:0 !important;border-bottom:3px dashed #eceef3 !important;padding:14px !important}
.t-stub span{display:inline !important;margin-right:10px !important}
.t-day{font-size:36px !important}
.t-ans a{display:block !important;margin:0 0 8px 0 !important;text-align:center !important}
}
</style></head>
<body style="margin:0;padding:0;background:#eceef3">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#eceef3"><tr>
<td class="t-pane" align="center" style="padding:36px 24px">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:560px"><tr><td>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#ffffff;border-radius:6px"><tr>
{{- with .Stub}}
<td class="t-stub" width="140" align="center" valign="middle" style="width:140px;padding:22px 10px;background:{{$.Accent}};color:#ffffff;font-family:'Courier New',Courier,monospace;text-align:center;border-right:3px dashed #eceef3;border-radius:6px 0 0 6px">
<span style="display:block;font-size:14px;letter-spacing:.2em">{{.Weekday}}</span>
<span class="t-day" style="display:block;font-size:52px;font-weight:700;line-height:1">{{.Day}}</span>
<span style="display:block;font-size:14px;letter-spacing:.2em">{{.Month}}</span>
{{- if .Time}}
<span style="display:block;margin-top:8px;font-size:15px;font-weight:700">{{.Time}}</span>
{{- end}}
</td>
{{- end}}
<td class="t-in" valign="top" style="padding:22px 24px;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:15px;line-height:1.5;color:#2b2f36">
{{- if .Campaign}}
<p style="margin:0 0 10px;font-size:12px;font-weight:700;letter-spacing:.12em;text-transform:uppercase;color:{{.Accent}}">{{.Campaign}}</p>
{{- end}}
<h1 style="margin:0 0 10px;font-size:21px;font-weight:700;line-height:1.25;color:#111111">{{.Title}}</h1>
{{- range .Lines}}
<p style="margin:0 0 10px">{{.}}</p>
{{- end}}
{{- range .Groups}}
<p style="margin:12px 0 0;font-weight:700;color:#111111">{{.Title}}</p>
{{- if .Detail}}
<p style="margin:0;font-size:14px;color:#5b616b">{{.Detail}}</p>
{{- end}}
<p class="t-ans" style="margin:8px 0 4px">{{range $i, $a := .Answers}}<a href="{{$a.URL}}" style="display:inline-block;margin:0 6px 6px 0;padding:0 16px;line-height:40px;border:2px solid #111111;border-radius:4px;font-size:14px;font-weight:700;text-decoration:none;{{if eq $i 0}}background:#111111;color:#ffffff{{else}}color:#111111{{end}}">{{$a.Label}}</a> {{end}}</p>
{{- end}}
{{- if .Answers}}
<p class="t-ans" style="margin:4px 0 10px">{{range $i, $a := .Answers}}<a href="{{$a.URL}}" style="display:inline-block;margin:0 6px 6px 0;padding:0 18px;line-height:40px;border:2px solid #111111;border-radius:4px;font-size:14px;font-weight:700;text-decoration:none;{{if eq $i 0}}background:#111111;color:#ffffff{{else}}color:#111111{{end}}">{{$a.Label}}</a> {{end}}</p>
{{- end}}
{{- if .Links}}
<p style="margin:0 0 10px;font-size:14px">{{range .Links}}<a href="{{.URL}}" style="color:{{$.Accent}};text-decoration:underline;margin-right:16px">{{.Label}}</a> {{end}}</p>
{{- end}}
{{- if .Small}}
<p style="margin:0;font-size:13px;color:#8a8f98">{{.Small}}</p>
{{- end}}
</td></tr></table>
{{- if .Why}}
<p style="margin:0;padding-top:12px;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:12px;line-height:1.5;color:#8a8f98;text-align:center">{{.Why}}{{if .PrefsURL}} <a href="{{.PrefsURL}}" style="color:#8a8f98;text-decoration:underline">Change what reaches you</a>{{end}}</p>
{{- end}}
</td></tr></table>
</td></tr></table>
</body></html>
`
