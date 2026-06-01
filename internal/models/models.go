package models

import "fmt"

type Route struct {
	PublicID string
	Agent    string
}

var Routes = []Route{
	{PublicID: "adina-analysis", Agent: "plan"},
	{PublicID: "adina-execution", Agent: "build"},
}

func Resolve(id string) (Route, bool) {
	for _, route := range Routes {
		if route.PublicID == id {
			return route, true
		}
	}
	return Route{}, false
}

func MustResolve(id string) Route {
	route, ok := Resolve(id)
	if !ok {
		panic(fmt.Sprintf("unknown model route %q", id))
	}
	return route
}
