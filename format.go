package aslconv

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/samber/lo"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

type Format int

const (
	FormatJSON Format = iota
	FormatHCL
	FormatDOT
	formatInvalid
)

func Formats() []Format {
	formats := make([]Format, 0, formatInvalid)
	for i := Format(0); i < formatInvalid; i++ {
		formats = append(formats, i)
	}
	return formats
}

func GetFormat(name string) (Format, bool) {
	switch strings.ToLower(name) {
	case "json":
		return FormatJSON, true
	case "hcl":
		return FormatHCL, true
	case "dot", "graphviz":
		return FormatDOT, true
	}
	return formatInvalid, false
}

func (f Format) String() string {
	switch f {
	case FormatJSON:
		return "JSON"
	case FormatHCL:
		return "HCL (HashiCorp configuration language)"
	case FormatDOT:
		return "DOT (text/vnd.graphviz, output only)"
	}
	return ""
}

func (f Format) Exts() []string {
	switch f {
	case FormatJSON:
		return []string{"*.json"}
	case FormatHCL:
		return []string{"*.hcl", "*.hcl.json"}
	case FormatDOT:
		return []string{"*.gv", "*.dot"}
	}
	return []string{}
}

type LoadOptions struct {
	HCLEvalContext                 *hcl.EvalContext
	HCLDiagnosticWriterInitializer func(*hclparse.Parser) hcl.DiagnosticWriter
}

func newLoadOptions() *LoadOptions {
	opts := &LoadOptions{
		HCLEvalContext: &hcl.EvalContext{
			Functions: map[string]function.Function{
				"jsonencode": stdlib.JSONEncodeFunc,
				"jsondecode": stdlib.JSONDecodeFunc,
			},
		},
		HCLDiagnosticWriterInitializer: func(parser *hclparse.Parser) hcl.DiagnosticWriter {
			return hcl.NewDiagnosticTextWriter(os.Stderr, parser.Files(), 400, true)
		},
	}
	return opts
}

func (opt *LoadOptions) apply(optFns ...func(*LoadOptions)) *LoadOptions {
	for _, optFn := range optFns {
		optFn(opt)
	}
	return opt
}

func LoadASLWithPath(path string, optFns ...func(*LoadOptions)) (*AmazonStatesLanguage, error) {
	format, err := DetectFormat(path)
	if err != nil {
		return nil, err
	}
	return format.LoadASLWithPath(path, optFns...)
}

func (f Format) LoadASLWithPath(path string, optFns ...func(*LoadOptions)) (*AmazonStatesLanguage, error) {
	opts := newLoadOptions().apply(optFns...)
	if stats, err := os.Stat(path); f == FormatHCL && err == nil && stats.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		parser := hclparse.NewParser()
		var diags hcl.Diagnostics
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			filename := filepath.Join(path, entry.Name())
			if ext := filepath.Ext(filename); ext != ".json" && ext != ".hcl" {
				continue
			}
			switch filepath.Ext(path) {
			case ".json":
				_, parseDiags := parser.ParseJSONFile(filename)
				diags = append(diags, parseDiags...)
			case ".hcl":
				_, parseDiags := parser.ParseHCLFile(filename)
				diags = append(diags, parseDiags...)
			default:
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Can not open file",
					Detail:   "unknown hcl format",
					Subject: &hcl.Range{
						Filename: filename,
					},
				})
			}
		}
		if diags.HasErrors() {
			return nil, convertDiagnosticsToError(diags, parser, opts)
		}
		body := hcl.MergeBodies(lo.Map(lo.Values(parser.Files()), func(file *hcl.File, _ int) hcl.Body {
			return file.Body
		}))
		return loadASLWithBody(body, opts)
	}
	switch f {
	default:
		bs, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return f.loadASLWithBytes(bs, path, opts)
	}
}

func LoadASLWithReader(reader io.Reader, formatName string, optFns ...func(*LoadOptions)) (*AmazonStatesLanguage, error) {
	format, ok := GetFormat(formatName)
	if !ok {
		return nil, fmt.Errorf("%s is unknown format", formatName)
	}
	return format.LoadASLWithReader(reader, optFns...)
}

func (f Format) LoadASLWithReader(reader io.Reader, optFns ...func(*LoadOptions)) (*AmazonStatesLanguage, error) {
	opts := newLoadOptions().apply(optFns...)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return f.loadASLWithBytes(data, "", opts)
}

func LoadASLWithBytes(data []byte, path string, optFns ...func(*LoadOptions)) (*AmazonStatesLanguage, error) {
	format, err := DetectFormat(path)
	if err != nil {
		return nil, err
	}
	return format.LoadASLWithBytes(data, path, optFns...)
}

func (f Format) LoadASLWithBytes(data []byte, path string, optFns ...func(*LoadOptions)) (*AmazonStatesLanguage, error) {
	opts := newLoadOptions().apply(optFns...)
	return f.loadASLWithBytes(data, path, opts)
}

func (f Format) loadASLWithBytes(data []byte, path string, opts *LoadOptions) (*AmazonStatesLanguage, error) {
	switch f {
	case FormatJSON:
		var asl AmazonStatesLanguage
		if err := json.Unmarshal(data, &asl); err != nil {
			return nil, err
		}
		return &asl, nil
	case FormatHCL:
		if path == "" {
			path = "asl.hcl"
		}
		parser := hclparse.NewParser()
		var file *hcl.File
		var diags hcl.Diagnostics
		switch filepath.Ext(path) {
		case ".json":
			file, diags = parser.ParseJSON(data, path)
		default:
			file, diags = parser.ParseHCL(data, path)
		}
		if diags.HasErrors() {
			return nil, convertDiagnosticsToError(diags, parser, opts)
		}
		asl, loadDiags := loadASLWithBody(file.Body, opts)
		diags = append(diags, loadDiags...)
		if diags.HasErrors() {
			return nil, convertDiagnosticsToError(diags, parser, opts)
		}
		return asl, convertDiagnosticsToError(diags, parser, opts)
	case FormatDOT:
		return nil, errors.New("DOT format is not support load file. this format support write only")
	}
	return nil, errors.New("unknown format")
}

func convertDiagnosticsToError(diags hcl.Diagnostics, parser *hclparse.Parser, opts *LoadOptions) error {
	if diags == nil {
		return nil
	}
	w := opts.HCLDiagnosticWriterInitializer(parser)
	if w == nil {
		return diags
	}
	if err := w.WriteDiagnostics(diags); err != nil {
		return err
	}
	if diags.HasErrors() {
		return diags
	}
	return nil
}

func loadASLWithBody(body hcl.Body, opts *LoadOptions) (*AmazonStatesLanguage, hcl.Diagnostics) {
	var asl AmazonStatesLanguage
	diags := asl.DecodeBody(body, opts.HCLEvalContext)
	return &asl, diags
}

func (f Format) WriteASL(writer io.Writer, asl *AmazonStatesLanguage) error {
	switch f {
	case FormatJSON:
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(asl)
	case FormatHCL:
		file := hclwrite.NewEmptyFile()
		if err := asl.EncodeBody(file.Body()); err != nil {
			return err
		}
		if _, err := file.WriteTo(writer); err != nil {
			return err
		}
		return nil
	case FormatDOT:
		dot, err := asl.MarshalDOT("G")
		if err != nil {
			return err
		}
		_, err = io.WriteString(writer, dot)
		return err
	}
	return errors.New("unknown format")
}

func ListFormat(w io.Writer) {
	for _, format := range Formats() {
		fmt.Fprintf(w, "%s [%s]\n", format, strings.Join(format.Exts(), ", "))
	}
}

func DetectFormat(path string) (Format, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return formatInvalid, err
	}
	if stat.IsDir() {
		return FormatHCL, nil
	}
	ext := filepath.Ext(path)
	switch ext {
	case "":
		return FormatHCL, nil
	case ".hcl":
		return FormatHCL, nil
	case ".json":
		switch filepath.Ext(strings.TrimSuffix(path, ext)) {
		case ".hcl":
			return FormatHCL, nil
		default:
			return FormatJSON, nil
		}
	case ".gv", ".dot":
		return FormatDOT, nil
	}
	return formatInvalid, errors.New("can not detect format")
}
