package email

// noteHTML is the Note layout: no picture and no box, text in a narrow
// column, and the campaign colour only on the links. Mail apps don't follow
// Chronicle's theme, so the colours are fixed and the page asks for light;
// inline styles carry the computer look, and the one media query turns the
// answers into full-width rows of at least 44px on a phone. Tables hold the
// column because Outlook ignores max-width on a div.
const noteHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light"><meta name="supported-color-schemes" content="light">
<title>{{.Title}}</title>
<style>
@media (max-width:600px){
.n-pane{padding:24px 20px !important}
.n-ans a{display:block !important;margin:0 0 8px 0 !important;padding:0 14px !important;line-height:46px !important;min-height:46px !important;border:1px solid #d9dbe1 !important;border-radius:10px !important;text-decoration:none !important}
}
</style></head>
<body style="margin:0;padding:0;background:#ffffff">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#ffffff"><tr>
<td class="n-pane" style="padding:40px 48px 32px">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:460px"><tr>
<td style="font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:16px;line-height:1.6;color:#2b2f36">
{{- if .Campaign}}
<p style="margin:0 0 14px;font-size:13px;color:#8a8f98">{{.Campaign}}</p>
{{- end}}
<h1 style="margin:0 0 14px;font-size:19px;font-weight:600;line-height:1.3;color:#111111">{{.Title}}</h1>
{{- range .Lines}}
<p style="margin:0 0 14px">{{.}}</p>
{{- end}}
{{- range .Groups}}
<p style="margin:0;font-weight:600;color:#111111">{{.Title}}</p>
{{- if .Detail}}
<p style="margin:0;font-size:14px;color:#5b616b">{{.Detail}}</p>
{{- end}}
<p class="n-ans" style="margin:6px 0 18px">{{range .Answers}}<a href="{{.URL}}" style="color:{{$.Accent}};text-decoration:underline;margin-right:18px">{{.Label}}</a> {{end}}</p>
{{- end}}
{{- if .Answers}}
<p class="n-ans" style="margin:0 0 14px">{{range .Answers}}<a href="{{.URL}}" style="color:{{$.Accent}};text-decoration:underline;margin-right:18px">{{.Label}}</a> {{end}}</p>
{{- end}}
{{- if .Links}}
<p style="margin:0 0 14px;font-size:14px">{{range .Links}}<a href="{{.URL}}" style="color:{{$.Accent}};text-decoration:underline;margin-right:18px">{{.Label}}</a> {{end}}</p>
{{- end}}
{{- if .Small}}
<p style="margin:0 0 14px;font-size:13px;color:#8a8f98">{{.Small}}</p>
{{- end}}
{{- if .Why}}
<p style="margin:0;padding-top:18px;font-size:12px;color:#a0a4ab">{{.Why}}{{if .PrefsURL}} <a href="{{.PrefsURL}}" style="color:#a0a4ab;text-decoration:underline">Change what reaches you</a>{{end}}</p>
{{- end}}
</td></tr></table>
</td></tr></table>
</body></html>
`
