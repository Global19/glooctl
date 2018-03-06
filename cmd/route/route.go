package route

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo-storage"
	"github.com/solo-io/gloo/pkg/protoutil"
	"github.com/spf13/pflag"

	"github.com/ghodss/yaml"
	"github.com/solo-io/gloo-api/pkg/api/types/v1"
	"github.com/solo-io/gloo-storage/file"
)

const (
	flagDomain      = "domain"
	flagVirtualHost = "virtual-host"
	flagFilename    = "filename"

	flagEvent         = "event"
	flagPathExact     = "path-exact"
	flagPathRegex     = "path-regex"
	flagPathPrefix    = "path-prefix"
	flagMethod        = "http-method"
	flagHeaders       = "header"
	flagUpstream      = "upstream"
	flagFunction      = "function"
	flagPrefixRewrite = "prefix-rewrite"

	flagKubeName      = "kube-upstream"
	flagKubeNamespace = "kube-namespace"
	flagKubePort      = "kube-port"

	defaultVHost = "default"

	upstreamTypeKubernetes = "kubernetes"
	kubeSpecName           = "service_name"
	kubeSpecNamespace      = "service_namespace"
	kubeSpecPort           = "service_port"
)

type kubeUpstream struct {
	name      string
	namespace string
	port      int
}

type routeDetail struct {
	event         string
	pathExact     string
	pathRegex     string
	pathPrefix    string
	verb          string
	headers       string
	upstream      string
	function      string
	prefixRewrite string

	kube kubeUpstream
}

func parseFile(filename string) (*v1.Route, error) {
	var r v1.Route
	err := file.ReadFileInto(filename, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func printRoutes(routes []*v1.Route, output string) {
	if len(routes) == 0 {
		fmt.Println("No routes defined")
		return
	}
	switch output {
	case "json":
		printJSONList(routes)
	case "yaml":
		printYAMLList(routes)
	default:
		printSummaryList(routes)
	}
}

func printJSON(r *v1.Route) {
	b, err := protoutil.Marshal(r)
	if err != nil {
		fmt.Println("unable to convert to JSON ", err)
		return
	}
	fmt.Println(string(b))
}

func printYAML(r *v1.Route) {
	jsn, err := protoutil.Marshal(r)
	if err != nil {
		fmt.Println("unable to marshal ", err)
		return
	}
	b, err := yaml.JSONToYAML(jsn)
	if err != nil {
		fmt.Println("unable to convert to YAML ", err)
		return
	}
	fmt.Println(string(b))
}

func printJSONList(routes []*v1.Route) {
	for _, r := range routes {
		printJSON(r)
	}
}

func printYAMLList(routes []*v1.Route) {
	for _, r := range routes {
		printYAML(r)
	}
}

func printSummaryList(r []*v1.Route) {
	for _, entry := range r {
		fmt.Println(toString(entry))
	}
}

const (
	event      = "event       : "
	pathExact  = "exact path  : "
	pathRegex  = "regex path  : "
	pathPrefix = "path prefix : "
	unknown    = "matcher     : unknown"
)

func toString(r *v1.Route) string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%s\n -> %s\n",
		matcherToString(r),
		destinationToString(r))
}

func matcherToString(r *v1.Route) string {
	switch m := r.GetMatcher().(type) {
	case *v1.Route_EventMatcher:
		return event + m.EventMatcher.EventType
	case *v1.Route_RequestMatcher:
		var path string
		switch p := m.RequestMatcher.GetPath().(type) {
		case *v1.RequestMatcher_PathExact:
			path = pathExact + p.PathExact
		case *v1.RequestMatcher_PathRegex:
			path = pathRegex + p.PathRegex
		case *v1.RequestMatcher_PathPrefix:
			path = pathPrefix + p.PathPrefix
		default:
			path = unknown
		}
		verb := ""
		if m.RequestMatcher.Verbs != nil {
			verb = fmt.Sprintf("\nmethods     : %v", m.RequestMatcher.Verbs)
		}
		headers := ""
		if m.RequestMatcher.Headers != nil {
			headers = fmt.Sprintf("\nheaders     : %v", m.RequestMatcher.Headers)
		}
		return path + verb + headers
	default:
		return unknown
	}
}

func destinationToString(r *v1.Route) string {
	single := r.GetSingleDestination()
	if single != nil {
		return upstreamToString(single.GetUpstream(), single.GetFunction())
	}

	multi := r.GetMultipleDestinations()
	if multi != nil {
		b := bytes.Buffer{}
		b.WriteString("[\n")
		for _, m := range multi {
			fmt.Fprintf(&b, "  %3d, %s\n", m.GetWeight(),
				upstreamToString(m.GetUpstream(), m.GetFunction()))
		}
		b.WriteString("]")
		return b.String()
	}

	return "unknown"
}

func upstreamToString(u *v1.UpstreamDestination, f *v1.FunctionDestination) string {
	if u != nil {
		return u.Name
	}

	if f != nil {
		return fmt.Sprintf("%s/%s", f.UpstreamName, f.FunctionName)
	}

	return "<no destintation specified>"
}

func route(flags *pflag.FlagSet, sc storage.Interface) (*v1.Route, error) {
	filename, _ := flags.GetString(flagFilename)
	if filename != "" {
		return parseFile(filename)
	}

	rd := routeDetails(flags)
	if rd.kube.name != "" {
		upstream, err := upstream(rd.kube, sc)
		if err != nil {
			return nil, err
		}
		rd.upstream = upstream.Name
	}
	return fromRouteDetail(rd)
}

func routeDetails(flags *pflag.FlagSet) *routeDetail {
	get := func(key string) string {
		v, _ := flags.GetString(key)
		return v
	}

	port, err := flags.GetInt(flagKubePort)
	if err != nil {
		port = 0
	}

	return &routeDetail{
		event:         get(flagEvent),
		pathExact:     get(flagPathExact),
		pathRegex:     get(flagPathRegex),
		pathPrefix:    get(flagPathPrefix),
		verb:          get(flagMethod),
		headers:       get(flagHeaders),
		upstream:      get(flagUpstream),
		function:      get(flagFunction),
		prefixRewrite: get(flagPrefixRewrite),

		kube: kubeUpstream{
			name:      get(flagKubeName),
			namespace: get(flagKubeNamespace),
			port:      port,
		},
	}
}

func fromRouteDetail(rd *routeDetail) (*v1.Route, error) {
	route := &v1.Route{}

	// matcher
	if rd.event != "" {
		route.Matcher = &v1.Route_EventMatcher{
			EventMatcher: &v1.EventMatcher{EventType: rd.event},
		}
	} else {
		var verbs []string
		if rd.verb != "" {
			verbs = strings.Split(strings.ToUpper(rd.verb), ",")
			for i, v := range verbs {
				verbs[i] = strings.TrimSpace(v)
			}
		}

		var headers map[string]string
		if rd.headers != "" {
			headers = make(map[string]string)
			entries := strings.Split(rd.headers, ",")
			for _, e := range entries {
				parts := strings.SplitN(e, ":", 2)
				if len(parts) != 2 {
					return nil, fmt.Errorf("unable to parse headers %s", rd.headers)
				}
				headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
		if rd.pathExact != "" {
			route.Matcher = &v1.Route_RequestMatcher{
				RequestMatcher: &v1.RequestMatcher{
					Path:    &v1.RequestMatcher_PathExact{PathExact: rd.pathExact},
					Verbs:   verbs,
					Headers: headers,
				},
			}
		} else if rd.pathRegex != "" {
			route.Matcher = &v1.Route_RequestMatcher{
				RequestMatcher: &v1.RequestMatcher{
					Path:    &v1.RequestMatcher_PathRegex{PathRegex: rd.pathRegex},
					Verbs:   verbs,
					Headers: headers,
				},
			}
		} else if rd.pathPrefix != "" {
			route.Matcher = &v1.Route_RequestMatcher{
				RequestMatcher: &v1.RequestMatcher{
					Path:    &v1.RequestMatcher_PathPrefix{PathPrefix: rd.pathPrefix},
					Verbs:   verbs,
					Headers: headers,
				},
			}
		} else {
			return nil, fmt.Errorf("a matcher wasn't specified")
		}
	}

	// prefix rewrite
	if rd.prefixRewrite != "" {
		route.PrefixRewrite = rd.prefixRewrite
	}

	// destination
	if rd.upstream == "" {
		return nil, fmt.Errorf("an upstream is necessary for specifying destination")
	}
	// currently only support single destination
	if rd.function != "" {
		route.SingleDestination = &v1.Destination{
			DestinationType: &v1.Destination_Function{
				Function: &v1.FunctionDestination{
					UpstreamName: rd.upstream,
					FunctionName: rd.function},
			},
		}
	} else if rd.upstream != "" {
		route.SingleDestination = &v1.Destination{
			DestinationType: &v1.Destination_Upstream{
				Upstream: &v1.UpstreamDestination{Name: rd.upstream},
			},
		}
	} else {
		return nil, fmt.Errorf("a destintation wasn't specified")
	}

	return route, nil
}

func upstream(kube kubeUpstream, sc storage.Interface) (*v1.Upstream, error) {
	upstreams, err := sc.V1().Upstreams().List()
	if err != nil {
		return nil, err
	}
	for _, u := range upstreams {
		if u.Type != upstreamTypeKubernetes {
			continue
		}
		s, _ := protoutil.MarshalMap(u.Spec)
		n, exists := s[kubeSpecName].(string)
		if !exists {
			continue
		}
		if n != kube.name {
			continue
		}
		if kube.namespace != "" {
			ns, exists := s[kubeSpecNamespace].(string)
			if !exists {
				continue
			}
			if ns != kube.namespace {
				continue
			}
		}

		if kube.port != 0 {
			p, exists := s[kubeSpecPort].(string)
			if !exists {
				continue
			}
			if p != strconv.Itoa(kube.port) {
				continue
			}
		}
		return u, nil
	}
	return nil, fmt.Errorf("unable to find kubernetes upstream %s/%s", kube.namespace, kube.name)
}
func createDefaultVHost(sc storage.Interface) error {
	vhost := &v1.VirtualHost{
		Name: defaultVHost,
	}
	_, err := sc.V1().VirtualHosts().Create(vhost)
	if err != nil && !storage.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func virtualHost(sc storage.Interface, vhostname, domain string, create bool) (*v1.VirtualHost, bool, error) {
	// make sure default virtual host exists
	if err := createDefaultVHost(sc); err != nil {
		return nil, false, err
	}

	if vhostname != "" {
		vh, err := sc.V1().VirtualHosts().Get(vhostname)
		if err != nil {
			return nil, false, err
		}
		return vh, false, nil
	}

	if domain == "" {
		vh, err := sc.V1().VirtualHosts().Get(defaultVHost)
		if err != nil {
			return nil, false, err
		}
		return vh, false, nil
	}

	// we are looking for specific vhost
	vhosts, err := sc.V1().VirtualHosts().List()
	if err != nil {
		return nil, false, errors.Wrap(err, "unable to get list of existing virtual hosts")
	}
	var validOnes []*v1.VirtualHost
	for _, v := range vhosts {
		if contains(v, domain) {
			validOnes = append(validOnes, v)
		}
	}
	switch len(validOnes) {
	case 0:
		if !create {
			return nil, false, fmt.Errorf("didn't find any virtual host for domain %s", domain)
		}
		created, err := sc.V1().VirtualHosts().Create(&v1.VirtualHost{
			Name:    domain,
			Domains: []string{domain},
		})
		if err != nil {
			return nil, false, err
		}
		return created, true, nil

	case 1:
		return validOnes[0], false, nil
	default:
		return nil, false, fmt.Errorf("the domain %s matched %d virtual hosts", domain, len(validOnes))
	}
}

func contains(vh *v1.VirtualHost, d string) bool {
	for _, e := range vh.Domains {
		if e == d {
			return true
		}
	}
	return false
}
