package email

// posterHTML is the Poster layout: a dark card with a band of the campaign
// colour across the top, the campaign in spaced gold capitals, the title in
// large serif type and the answers as pill buttons, the first one filled
// because it is the answer most people give. The band is painted, not a
// picture: a picture in an email would need a media link that never expires
// and opens without signing in. Mail apps that drop gradients show the
// band's flat colour. On a phone the buttons become full-width rows.
const posterHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light"><meta name="supported-color-schemes" content="light">
<title>{{.Title}}</title>
<style>
@media (max-width:600px){
.p-pane{padding:0 !important}
.p-in{padding:6px 20px 24px !important}
.p-h{font-size:26px !important}
.p-ans a{display:block !important;margin:0 0 10px 0 !important}
}
</style></head>
<body style="margin:0;padding:0;background:#0f1420">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#0f1420"><tr>
<td class="p-pane" align="center" style="padding:36px 0">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:520px;background:#161c2b"><tr>
<td height="150" style="height:150px;line-height:150px;font-size:0;background-color:{{.Accent}};background-image:linear-gradient(135deg,#c9a86a 0%,{{.Accent}} 38%,#2b3a55 72%,#161c2b 100%)">&nbsp;</td>
</tr><tr>
<td class="p-in" align="center" style="padding:10px 36px 30px;font-family:Georgia,'Times New Roman',serif;font-size:16px;line-height:1.55;color:#d8dbe4;text-align:center">
{{- if .Campaign}}
<p style="margin:0 0 12px;font-size:13px;letter-spacing:.3em;text-transform:uppercase;color:#c9a86a">{{.Campaign}}</p>
{{- end}}
<h1 class="p-h" style="margin:0 0 12px;font-size:30px;font-weight:normal;line-height:1.15;color:#f4efe4">{{.Title}}</h1>
{{- range .Lines}}
<p style="margin:0 0 12px">{{.}}</p>
{{- end}}
{{- range .Groups}}
<p style="margin:14px 0 0;color:#f4efe4">{{.Title}}</p>
{{- if .Detail}}
<p style="margin:0;font-size:14px;color:#9aa1b2">{{.Detail}}</p>
{{- end}}
<p class="p-ans" style="margin:10px 0 6px;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif">{{range $i, $a := .Answers}}<a href="{{$a.URL}}" style="display:inline-block;margin:0 4px 8px;padding:0 20px;line-height:42px;border-radius:999px;border:1px solid #c9a86a;font-size:15px;font-weight:600;text-decoration:none;{{if eq $i 0}}background:#c9a86a;color:#161c2b{{else}}color:#f4efe4{{end}}">{{$a.Label}}</a> {{end}}</p>
{{- end}}
{{- if .Answers}}
<p class="p-ans" style="margin:6px 0 12px;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif">{{range $i, $a := .Answers}}<a href="{{$a.URL}}" style="display:inline-block;margin:0 4px 8px;padding:0 22px;line-height:44px;border-radius:999px;border:1px solid #c9a86a;font-size:15px;font-weight:600;text-decoration:none;{{if eq $i 0}}background:#c9a86a;color:#161c2b{{else}}color:#f4efe4{{end}}">{{$a.Label}}</a> {{end}}</p>
{{- end}}
{{- if .Links}}
<p style="margin:0 0 12px;font-size:14px;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif">{{range .Links}}<a href="{{.URL}}" style="color:#c9a86a;text-decoration:underline;margin:0 9px">{{.Label}}</a> {{end}}</p>
{{- end}}
{{- if .Small}}
<p style="margin:0;font-size:13px;color:#9aa1b2;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif">{{.Small}}</p>
{{- end}}
</td></tr>
{{- if .Why}}
<tr><td align="center" style="padding:14px 20px 18px;font-family:-apple-system,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:12px;line-height:1.5;color:#7d8496;text-align:center">{{.Why}}{{if .PrefsURL}} <a href="{{.PrefsURL}}" style="color:#9aa1b2;text-decoration:underline">Change what reaches you</a>{{end}}</td></tr>
{{- end}}
</table>
</td></tr></table>
</body></html>
`
