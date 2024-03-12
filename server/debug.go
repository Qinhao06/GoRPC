package server

import (
	"GoRpc/service"
	"html/template"
	"net/http"
)

const debugText = `<html>
	<body>
	<title>GeeRPC Services</title>
	{{range .}}
	<hr>
	Service {{.Name}}
	<hr>
		<table>
		<th align=center>Method</th><th align=center>Calls</th>
		{{range $name, $mType := .Method}}
			<tr>
			<td align=left font=fixed>{{$name}}({{$mType.ArgType}}, {{$mType.ReplyType}}) error</td>
			<td align=center>{{$mType.NumCalls}}</td>
			</tr>
		{{end}}
		</table>
	{{end}}
	</body>
	</html>`

var debug = template.Must(template.New("debug").Parse(debugText))

type debugHttp struct {
	*Server
}

type debugService struct {
	Name    string
	Methods map[string]*service.MethodType
}

func (s debugHttp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var services []debugService
	s.serviceMap.Range(func(k, v interface{}) bool {
		svc := v.(*service.Service)
		services = append(services, debugService{
			Name:    k.(string),
			Methods: svc.Methods,
		})
		return true
	})

	err := debug.Execute(w, services)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
