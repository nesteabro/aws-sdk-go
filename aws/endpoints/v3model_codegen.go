//go:build codegen
// +build codegen

package endpoints

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"text/template"
	"unicode"
)

// A CodeGenOptions are the options for code generating the endpoints into
// Go code from the endpoints model definition.
type CodeGenOptions struct {
	// Options for how the model will be decoded.
	DecodeModelOptions DecodeModelOptions

	// Disables code generation of the service endpoint prefix IDs defined in
	// the model.
	DisableGenerateServiceIDs bool
}

// Set combines all of the option functions together
func (d *CodeGenOptions) Set(optFns ...func(*CodeGenOptions)) {
	for _, fn := range optFns {
		fn(d)
	}
}

// CodeGenModel given a endpoints model file will decode it and attempt to
// generate Go code from the model definition. Error will be returned if
// the code is unable to be generated, or decoded.
func CodeGenModel(modelFile io.Reader, outFile io.Writer, optFns ...func(*CodeGenOptions)) error {
	var opts CodeGenOptions
	opts.Set(optFns...)

	resolver, err := DecodeModel(modelFile, func(d *DecodeModelOptions) {
		*d = opts.DecodeModelOptions
	})
	if err != nil {
		return err
	}

	v := struct {
		Resolver
		CodeGenOptions
	}{
		Resolver:       resolver,
		CodeGenOptions: opts,
	}

	tmpl := template.Must(template.New("tmpl").Funcs(funcMap).Parse(v3Tmpl))
	if err := tmpl.ExecuteTemplate(outFile, "defaults", v); err != nil {
		return fmt.Errorf("failed to execute template, %v", err)
	}

	return nil
}

func toSymbol(v string) string {
	out := []rune{}
	for _, c := range strings.Title(v) {
		if !(unicode.IsNumber(c) || unicode.IsLetter(c)) {
			continue
		}

		out = append(out, c)
	}

	return string(out)
}

func quoteString(v string) string {
	return fmt.Sprintf("%q", v)
}

func regionConstName(p, r string) string {
	return toSymbol(p) + toSymbol(r)
}

func partitionGetter(id string) string {
	return fmt.Sprintf("%sPartition", toSymbol(id))
}

func partitionVarName(id string) string {
	return fmt.Sprintf("%sPartition", strings.ToLower(toSymbol(id)))
}

func listPartitionNames(ps partitions) string {
	names := []string{}
	switch len(ps) {
	case 1:
		return ps[0].Name
	case 2:
		return fmt.Sprintf("%s and %s", ps[0].Name, ps[1].Name)
	default:
		for i, p := range ps {
			if i == len(ps)-1 {
				names = append(names, "and "+p.Name)
			} else {
				names = append(names, p.Name)
			}
		}
		return strings.Join(names, ", ")
	}
}

func boxedBoolIfSet(msg string, v boxedBool) string {
	switch v {
	case boxedTrue:
		return fmt.Sprintf(msg, "boxedTrue")
	case boxedFalse:
		return fmt.Sprintf(msg, "boxedFalse")
	default:
		return ""
	}
}

func stringIfSet(msg, v string) string {
	if len(v) == 0 {
		return ""
	}

	return fmt.Sprintf(msg, v)
}

func stringSliceIfSet(msg string, vs []string) string {
	if len(vs) == 0 {
		return ""
	}

	names := []string{}
	for _, v := range vs {
		names = append(names, `"`+v+`"`)
	}

	return fmt.Sprintf(msg, strings.Join(names, ","))
}

func endpointIsSet(v endpoint) bool {
	return !reflect.DeepEqual(v, endpoint{})
}

func serviceSet(ps partitions) map[string]struct{} {
	set := map[string]struct{}{}
	for _, p := range ps {
		for id := range p.Services {
			set[id] = struct{}{}
		}
	}

	return set
}

func endpointVariantSetter(variant endpointVariant) (string, error) {
	if variant == 0 {
		return "0", nil
	}

	if variant > (fipsVariant | dualStackVariant) {
		return "", fmt.Errorf("unknown endpoint variant")
	}

	var symbols []string
	if variant&fipsVariant != 0 {
		symbols = append(symbols, "fipsVariant")
	}
	if variant&dualStackVariant != 0 {
		symbols = append(symbols, "dualStackVariant")
	}
	v := strings.Join(symbols, "|")

	return v, nil
}

func endpointKeySetter(e endpointKey) (string, error) {
	var sb strings.Builder
	sb.WriteString("endpointKey{\n")
	sb.WriteString(fmt.Sprintf("Region: %q,\n", e.Region))
	if e.Variant != 0 {
		variantSetter, err := endpointVariantSetter(e.Variant)
		if err != nil {
			return "", err
		}
		sb.WriteString(fmt.Sprintf("Variant: %s,\n", variantSetter))
	}
	sb.WriteString("}")
	return sb.String(), nil
}

func defaultKeySetter(e defaultKey) (string, error) {
	var sb strings.Builder
	sb.WriteString("defaultKey{\n")
	if e.Variant != 0 {
		variantSetter, err := endpointVariantSetter(e.Variant)
		if err != nil {
			return "", err
		}
		sb.WriteString(fmt.Sprintf("Variant: %s,\n", variantSetter))
	}
	sb.WriteString("}")
	return sb.String(), nil
}

var funcMap = template.FuncMap{
	"ToSymbol":              toSymbol,
	"QuoteString":           quoteString,
	"RegionConst":           regionConstName,
	"PartitionGetter":       partitionGetter,
	"PartitionVarName":      partitionVarName,
	"ListPartitionNames":    listPartitionNames,
	"BoxedBoolIfSet":        boxedBoolIfSet,
	"StringIfSet":           stringIfSet,
	"StringSliceIfSet":      stringSliceIfSet,
	"EndpointIsSet":         endpointIsSet,
	"ServicesSet":           serviceSet,
	"EndpointVariantSetter": endpointVariantSetter,
	"EndpointKeySetter":     endpointKeySetter,
	"DefaultKeySetter":      defaultKeySetter,
}

const v3Tmpl = `
{{ define "defaults" -}}
// Code generated by aws/endpoints/v3model_codegen.go. DO NOT EDIT.

package endpoints

import (
	"regexp"
)

	{{ template "partition consts" $.Resolver }}

	{{ range $_, $partition := $.Resolver }}
		{{ template "partition region consts" $partition }}
	{{ end }}

	{{ if not $.DisableGenerateServiceIDs -}}
	{{ template "service consts" $.Resolver }}
	{{- end }}
	
	{{ template "endpoint resolvers" $.Resolver }}
{{- end }}

{{ define "partition consts" }}
	// Partition identifiers
	const (
		{{ range $_, $p := . -}}
			{{ ToSymbol $p.ID }}PartitionID = {{ QuoteString $p.ID }} // {{ $p.Name }} partition.
		{{ end -}}
	)
{{- end }}

{{ define "partition region consts" }}
	// {{ .Name }} partition's regions.
	const (
		{{ range $id, $region := .Regions -}}
			{{ ToSymbol $id }}RegionID = {{ QuoteString $id }} // {{ $region.Description }}.
		{{ end -}}
	)
{{- end }}

{{ define "service consts" }}
	// Service identifiers
	const (
		{{ $serviceSet := ServicesSet . -}}
		{{ range $id, $_ := $serviceSet -}}
			{{ ToSymbol $id }}ServiceID = {{ QuoteString $id }} // {{ ToSymbol $id }}.
		{{ end -}}
	)
{{- end }}

{{ define "endpoint resolvers" }}
	// DefaultResolver returns an Endpoint resolver that will be able
	// to resolve endpoints for: {{ ListPartitionNames . }}.
	//
	// Use DefaultPartitions() to get the list of the default partitions.
	func DefaultResolver() Resolver {
		return defaultPartitions
	}

	// DefaultPartitions returns a list of the partitions the SDK is bundled
	// with. The available partitions are: {{ ListPartitionNames . }}.
	//
	//	partitions := endpoints.DefaultPartitions
	//	for _, p := range partitions {
	//	    // ... inspect partitions
	//	}
	func DefaultPartitions() []Partition {
		return defaultPartitions.Partitions()
	}

	var defaultPartitions = partitions{
		{{ range $_, $partition := . -}}
			{{ PartitionVarName $partition.ID }},
		{{ end }}
	}
	
	{{ range $_, $partition := . -}}
		{{ $name := PartitionGetter $partition.ID -}}
		// {{ $name }} returns the Resolver for {{ $partition.Name }}.
		func {{ $name }}() Partition {
			return  {{ PartitionVarName $partition.ID }}.Partition()
		}
		var {{ PartitionVarName $partition.ID }} = {{ template "gocode Partition" $partition }}
	{{ end }}
{{ end }}

{{ define "default partitions" }}
	func DefaultPartitions() []Partition {
		return []partition{
			{{ range $_, $partition := . -}}
			// {{ ToSymbol $partition.ID}}Partition(),
			{{ end }}
		}
	}
{{ end }}

{{ define "gocode Partition" -}}
partition{
	{{ StringIfSet "ID: %q,\n" .ID -}}
	{{ StringIfSet "Name: %q,\n" .Name -}}
	{{ StringIfSet "DNSSuffix: %q,\n" .DNSSuffix -}}
	RegionRegex: {{ template "gocode RegionRegex" .RegionRegex }},
	{{ if (gt (len .Defaults) 0) -}}
		Defaults: {{ template "gocode Defaults" .Defaults -}},
	{{ end -}}
	Regions:  {{ template "gocode Regions" .Regions }},
	Services: {{ template "gocode Services" .Services }},
}
{{- end }}

{{ define "gocode RegionRegex" -}}
regionRegex{
	Regexp: func() *regexp.Regexp{
		reg, _ := regexp.Compile({{ QuoteString .Regexp.String }})
		return reg
	}(),
}
{{- end }}

{{ define "gocode Regions" -}}
regions{
	{{ range $id, $region := . -}}
		"{{ $id }}": {{ template "gocode Region" $region }},
	{{ end -}}
}
{{- end }}

{{ define "gocode Region" -}}
region{
	{{ StringIfSet "Description: %q,\n" .Description -}}
}
{{- end }}

{{ define "gocode Services" -}}
services{
	{{ range $id, $service := . -}}
	"{{ $id }}": {{ template "gocode Service" $service }},
	{{ end }}
}
{{- end }}

{{ define "gocode Service" -}}
service{
	{{ StringIfSet "PartitionEndpoint: %q,\n" .PartitionEndpoint -}}
	{{ BoxedBoolIfSet "IsRegionalized: %s,\n" .IsRegionalized -}}
	{{ if (gt (len .Defaults) 0) -}}
		Defaults: {{ template "gocode Defaults" .Defaults -}},
	{{ end -}}
	{{ if .Endpoints -}}
		Endpoints: {{ template "gocode Endpoints" .Endpoints }},
	{{- end }}
}
{{- end }}

{{ define "gocode Defaults" -}}
endpointDefaults{
	{{ range $id, $endpoint := . -}}
	{{ DefaultKeySetter $id }}: {{ template "gocode Endpoint" $endpoint }},
	{{ end }}
}
{{- end }}

{{ define "gocode Endpoints" -}}
serviceEndpoints{
	{{ range $id, $endpoint := . -}}
	{{ EndpointKeySetter $id }}: {{ template "gocode Endpoint" $endpoint }},
	{{ end }}
}
{{- end }}

{{ define "gocode Endpoint" -}}
endpoint{
	{{ StringIfSet "Hostname: %q,\n" .Hostname -}}
	{{ StringIfSet "DNSSuffix: %q,\n" .DNSSuffix -}}
	{{ StringIfSet "SSLCommonName: %q,\n" .SSLCommonName -}}
	{{ StringSliceIfSet "Protocols: []string{%s},\n" .Protocols -}}
	{{ StringSliceIfSet "SignatureVersions: []string{%s},\n" .SignatureVersions -}}
	{{ if or .CredentialScope.Region .CredentialScope.Service -}}
	CredentialScope: credentialScope{
		{{ StringIfSet "Region: %q,\n" .CredentialScope.Region -}}
		{{ StringIfSet "Service: %q,\n" .CredentialScope.Service -}}
	},
	{{- end }}
	{{ BoxedBoolIfSet "Deprecated: %s,\n" .Deprecated -}}
}
{{- end }}
`
