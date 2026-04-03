package cli

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// BuildFlagSet 根据 Req 的 struct tag 构建 flag.FlagSet（不执行 Parse）。
// 可用于打印帮助信息：BuildFlagSet[Req]("app").Usage()。
func BuildFlagSet[Req any](name string) (*flag.FlagSet, error) {
	var zero Req
	rv := reflect.ValueOf(&zero).Elem()
	if rv.Kind() == reflect.Ptr {
		rv.Set(reflect.New(rv.Type().Elem()))
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("cli.BuildFlagSet: Req must be a struct (or pointer to struct), got %s", rv.Kind())
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	if _, err := collectFields(fs, rv, ""); err != nil {
		return nil, err
	}
	return fs, nil
}

// ParseFlags 通过反射将 args 解析并填充到 Req 结构体。
// Req 为非指针类型时直接使用；为指针类型时请用 ParseFlagsPtr。
//
// Flag 名称解析优先级：cli tag > json tag > 小写字段名。
//
// Tag 格式（`cli:"..."`），各修饰符顺序无关（usage= 必须最后）：
//
//	cli:"name"                                        — flag 名称
//	cli:"name,required"                               — 必填
//	cli:"name,default=30s"                            — 默认值
//	cli:"name,required,default=./out,usage=some text" — 组合；usage= 后内容可含逗号
//	cli:"-"                                           — 跳过该字段
//
// 若字段无 cli tag，则尝试读取 json tag 作为 flag 名（忽略 omitempty；json:"-" 视为跳过）。
//
// 支持类型：string、bool、int、int64、float64、time.Duration、[]string、
// map[string]string、map[string]int、map[string]int64、map[string]float64、map[string]bool。
//
// []string：flag 可重复传，-flag a -flag b → []string{"a","b"}。
// map：flag 可重复传，-flag k=v -flag k2=v2；value 按目标类型解析。
//
// 嵌套结构体：父字段 tag 名作为前缀，子 flag 用 "." 拼接，如 -download.dest。
func ParseFlags[Req any](args []string) (Req, error) {
	var zero Req
	return parseFlags[Req](args, reflect.ValueOf(&zero).Elem())
}

// ParseFlagsPtr 与 ParseFlags 相同，但 Req 本身是指针类型（如 *MyReq）。
func ParseFlagsPtr[Req any](args []string) (Req, error) {
	var zero Req
	rv := reflect.ValueOf(&zero).Elem()
	if rv.Kind() == reflect.Ptr {
		rv.Set(reflect.New(rv.Type().Elem()))
		return parseFlags[Req](args, rv.Elem())
	}
	return parseFlags[Req](args, rv)
}

type fieldMeta struct {
	name       string
	usage      string
	required   bool
	defVal     string
	hasDefault bool
	value      reflect.Value
}

func parseFlags[Req any](args []string, rv reflect.Value) (Req, error) {
	var zero Req

	if rv.Kind() != reflect.Struct {
		return zero, fmt.Errorf("cli.ParseFlags: Req must be a struct (or pointer to struct), got %s", rv.Kind())
	}

	fs := flag.NewFlagSet("app", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	fields, err := collectFields(fs, rv, "")
	if err != nil {
		return zero, err
	}

	if err := fs.Parse(args); err != nil {
		return zero, err
	}

	for _, fm := range fields {
		if fm.required && isZero(fm.value) {
			return zero, fmt.Errorf("-%s is required", fm.name)
		}
	}

	result := reflect.ValueOf(&zero).Elem()
	if result.Kind() == reflect.Ptr {
		result.Set(rv.Addr())
	} else {
		result.Set(rv)
	}
	return zero, nil
}

func collectFields(fs *flag.FlagSet, rv reflect.Value, prefix string) ([]fieldMeta, error) {
	rt := rv.Type()
	var fields []fieldMeta

	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		fv := rv.Field(i)

		if !fv.CanSet() {
			continue
		}

		skip, fieldName := resolveFlagName(sf)
		if skip {
			continue
		}

		qualifiedName := fieldName
		if prefix != "" {
			qualifiedName = prefix + "." + fieldName
		}

		if fv.Kind() == reflect.Struct && fv.Type() != reflect.TypeOf(time.Duration(0)) {
			nested, err := collectFields(fs, fv, qualifiedName)
			if err != nil {
				return nil, err
			}
			fields = append(fields, nested...)
			continue
		}

		fm, err := parseTag(sf, fv, qualifiedName)
		if err != nil {
			return nil, err
		}
		if err := bindFlag(fs, fm); err != nil {
			return nil, fmt.Errorf("field %s: %w", sf.Name, err)
		}
		fields = append(fields, *fm)
	}
	return fields, nil
}

// resolveFlagName 按优先级解析 flag 名称：cli tag > json tag > 小写字段名。
// 返回 (skip=true, "") 表示该字段应跳过。
func resolveFlagName(sf reflect.StructField) (skip bool, name string) {
	if cli, ok := sf.Tag.Lookup("cli"); ok {
		if cli == "-" {
			return true, ""
		}
		if first := strings.TrimSpace(strings.SplitN(cli, ",", 2)[0]); first != "" {
			return false, first
		}
		return false, strings.ToLower(sf.Name)
	}

	if j, ok := sf.Tag.Lookup("json"); ok {
		parts := strings.SplitN(j, ",", 2)
		n := strings.TrimSpace(parts[0])
		if n == "-" {
			return true, ""
		}
		if n != "" {
			return false, n
		}
	}

	return false, strings.ToLower(sf.Name)
}

// parseTag 解析 cli struct tag 中的修饰符（required / default= / usage=）。
// usage= 必须是最后一个修饰符，其值可以包含逗号。
func parseTag(sf reflect.StructField, fv reflect.Value, qualifiedName string) (*fieldMeta, error) {
	fm := &fieldMeta{name: qualifiedName, value: fv}

	raw, ok := sf.Tag.Lookup("cli")
	if !ok {
		return fm, nil
	}

	parts, usageVal := splitUsage(raw)
	fm.usage = usageVal

	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		switch {
		case p == "required":
			fm.required = true
		case strings.HasPrefix(p, "default="):
			fm.defVal = strings.TrimPrefix(p, "default=")
			fm.hasDefault = true
		}
	}
	return fm, nil
}

// splitUsage 找到第一个 ",usage=" 并将其后的所有内容作为 usage 文本返回。
func splitUsage(raw string) ([]string, string) {
	const marker = ",usage="
	idx := strings.Index(raw, marker)
	if idx < 0 {
		return strings.Split(raw, ","), ""
	}
	return strings.Split(raw[:idx], ","), raw[idx+len(marker):]
}

func bindFlag(fs *flag.FlagSet, fm *fieldMeta) error {
	fv := fm.value
	name, usage := fm.name, fm.usage

	switch fv.Kind() {
	case reflect.String:
		def := fv.String()
		if fm.hasDefault {
			def = fm.defVal
		}
		fs.Func(name, usage, func(s string) error { fv.SetString(s); return nil })
		if def != "" {
			if err := fs.Set(name, def); err != nil {
				return fmt.Errorf("invalid default value %q for flag -%s: %w", def, name, err)
			}
		}

	case reflect.Bool:
		def := fv.Bool()
		if fm.hasDefault {
			def = fm.defVal == "true"
		}
		fs.BoolVar((*bool)(fv.Addr().UnsafePointer()), name, def, usage)

	case reflect.Int:
		def := int(fv.Int())
		if fm.hasDefault {
			if _, err := fmt.Sscanf(fm.defVal, "%d", &def); err != nil {
				return fmt.Errorf("invalid default value %q for int flag -%s: %w", fm.defVal, name, err)
			}
		}
		fs.IntVar((*int)(fv.Addr().UnsafePointer()), name, def, usage)

	case reflect.Int64:
		if fv.Type() == reflect.TypeOf(time.Duration(0)) {
			def := time.Duration(fv.Int())
			if fm.hasDefault {
				d, err := time.ParseDuration(fm.defVal)
				if err != nil {
					return fmt.Errorf("invalid default value %q for duration flag -%s: %w", fm.defVal, name, err)
				}
				def = d
			}
			fs.DurationVar((*time.Duration)(fv.Addr().UnsafePointer()), name, def, usage)
		} else {
			def := fv.Int()
			if fm.hasDefault {
				if _, err := fmt.Sscanf(fm.defVal, "%d", &def); err != nil {
					return fmt.Errorf("invalid default value %q for int64 flag -%s: %w", fm.defVal, name, err)
				}
			}
			fs.Int64Var((*int64)(fv.Addr().UnsafePointer()), name, def, usage)
		}

	case reflect.Float64:
		def := fv.Float()
		if fm.hasDefault {
			if _, err := fmt.Sscanf(fm.defVal, "%f", &def); err != nil {
				return fmt.Errorf("invalid default value %q for float64 flag -%s: %w", fm.defVal, name, err)
			}
		}
		fs.Float64Var((*float64)(fv.Addr().UnsafePointer()), name, def, usage)

	case reflect.Slice:
		if fv.Type() != reflect.TypeOf([]string(nil)) {
			return fmt.Errorf("unsupported slice type %s for flag %q (only []string supported)", fv.Type(), name)
		}
		fs.Func(name, usage, func(s string) error {
			fv.Set(reflect.Append(fv, reflect.ValueOf(s)))
			return nil
		})
		if fm.hasDefault && fm.defVal != "" {
			for _, v := range strings.Split(fm.defVal, ",") {
				if t := strings.TrimSpace(v); t != "" {
					fv.Set(reflect.Append(fv, reflect.ValueOf(t)))
				}
			}
		}

	case reflect.Map:
		if err := bindMapFlag(fs, fm, fv, name, usage); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unsupported field type %s for flag %q", fv.Type(), name)
	}
	return nil
}

func bindMapFlag(fs *flag.FlagSet, fm *fieldMeta, fv reflect.Value, name, usage string) error {
	vt := fv.Type().Elem()
	switch vt.Kind() {
	case reflect.String, reflect.Int, reflect.Int64, reflect.Float64, reflect.Bool:
	default:
		return fmt.Errorf("unsupported map value type %s for flag %q", vt, name)
	}
	if fv.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("unsupported map key type %s for flag %q (only string keys supported)", fv.Type().Key(), name)
	}

	initMap := func() {
		if fv.IsNil() {
			fv.Set(reflect.MakeMap(fv.Type()))
		}
	}

	parseVal := func(s string) (reflect.Value, error) {
		switch vt.Kind() {
		case reflect.String:
			return reflect.ValueOf(s), nil
		case reflect.Int:
			n, err := strconv.Atoi(s)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("expected int, got %q", s)
			}
			return reflect.ValueOf(n).Convert(vt), nil
		case reflect.Int64:
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("expected int64, got %q", s)
			}
			return reflect.ValueOf(n).Convert(vt), nil
		case reflect.Float64:
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("expected float64, got %q", s)
			}
			return reflect.ValueOf(f).Convert(vt), nil
		case reflect.Bool:
			b, err := strconv.ParseBool(s)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("expected bool, got %q", s)
			}
			return reflect.ValueOf(b), nil
		}
		return reflect.Value{}, fmt.Errorf("unsupported type %s", vt)
	}

	fs.Func(name, usage, func(s string) error {
		k, raw, ok := strings.Cut(s, "=")
		if !ok {
			return fmt.Errorf("flag -%s: expected key=value, got %q", name, s)
		}
		v, err := parseVal(raw)
		if err != nil {
			return fmt.Errorf("flag -%s value: %w", name, err)
		}
		initMap()
		fv.SetMapIndex(reflect.ValueOf(k), v)
		return nil
	})

	if fm.hasDefault && fm.defVal != "" {
		initMap()
		for _, pair := range strings.Split(fm.defVal, ",") {
			k, raw, ok := strings.Cut(strings.TrimSpace(pair), "=")
			if !ok {
				continue
			}
			v, err := parseVal(raw)
			if err != nil {
				return fmt.Errorf("invalid default value for map flag -%s key %q: %w", name, k, err)
			}
			fv.SetMapIndex(reflect.ValueOf(k), v)
		}
	}
	return nil
}

func isZero(fv reflect.Value) bool {
	switch fv.Kind() {
	case reflect.String:
		return fv.String() == ""
	case reflect.Bool:
		return !fv.Bool()
	case reflect.Int, reflect.Int64:
		return fv.Int() == 0
	case reflect.Float64:
		return fv.Float() == 0
	case reflect.Slice, reflect.Map:
		return fv.Len() == 0
	}
	return false
}
