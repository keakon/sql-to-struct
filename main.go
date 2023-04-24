package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"sync"
	"unsafe"
)

type OutputType uint8

const (
	sqlTable OutputType = iota
	sqlTableWithJSON
	sbTable
)

var (
	defaultOutputTypes = []OutputType{sqlTableWithJSON, sbTable}

	pattern1 = regexp.MustCompile("(?s)create table if not exists `(\\w+)`(.+?)\\)\\s*engine") // 匹配多行
	pattern2 = regexp.MustCompile("`(\\w+)`\\s+(\\w+)\\s*(\\w+)?")
	pattern3 = "(?s)create table if not exists `(%s)`(.+?)\\) engine"

	pool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 1024))
		},
	}

	upperWords  = []string{"id", "dns", "uuid", "ldap", "ad", "dn", "sha256", "md5", "sso", "otp", "totp", "os", "sdk", "ipa"}
	transformer = map[string]string{
		"ips":    "IPs",
		"mysql":  "MySQL",
		"sqlite": "SQLite",
		"userid": "UserID",
	}
)

func init() {
	for _, k := range upperWords {
		transformer[k] = strings.ToUpper(k)
	}
}

func exitWithUsage() {
	fmt.Println("Usage: go run . <file_path> [<table_name>] [mode=sql/json/sb/sql+sb/...]")
	os.Exit(1)
}

func parseMode(mode string) []OutputType {
	if mode[:5] == "mode=" {
		mode = mode[5:]
		modes := strings.Split(mode, "+")
		outputTypes := []OutputType{}
		for _, mode := range modes {
			switch mode {
			case "sql":
				outputTypes = append(outputTypes, sqlTable)
			case "json":
				outputTypes = append(outputTypes, sqlTableWithJSON)
			case "sb":
				outputTypes = append(outputTypes, sbTable)
			}
		}
		if len(outputTypes) > 0 {
			return outputTypes
		}
	}
	return defaultOutputTypes
}

func main() {
	argCount := len(os.Args)

	if argCount < 2 {
		exitWithUsage()
	}

	var tableName string
	outputTypes := defaultOutputTypes
	if argCount >= 3 {
		arg := os.Args[2]
		if strings.Contains(arg, "=") {
			if argCount == 4 {
				exitWithUsage()
			}
			outputTypes = parseMode(arg)
		} else {
			tableName = arg
			if argCount >= 4 {
				outputTypes = parseMode(os.Args[3])
			}
		}
	}

	content, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	tables := parseSQL(content, tableName)
	for _, table := range tables {
		for _, outputType := range outputTypes {
			fmt.Println(table.ToString(outputType))
		}
	}
}

func bytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func parseSQL(sql []byte, tableName string) (tables []Table) {
	sql = bytes.ToLower(sql)
	s := bytesToString(sql)

	if tableName == "" {
		matches := pattern1.FindAllStringSubmatch(s, -1)
		for _, match := range matches {
			tables = append(tables, parseTable(match))
		}
	} else {
		p := regexp.MustCompile(fmt.Sprintf(pattern3, tableName))
		match := p.FindStringSubmatch(s)
		tables = append(tables, parseTable(match))
	}
	return
}

func parseTable(match []string) Table {
	name := match[1]
	table := newTable(name)
	columnsContent := match[2]
	subMatches := pattern2.FindAllStringSubmatch(columnsContent, -1)
	for _, m := range subMatches {
		column := newColumn(m[1], m[2], m[3] == "unsigned")
		table.Columns = append(table.Columns, column)
	}
	return table
}

type Table struct {
	RawName string
	Name    string
	Columns []Column
}

func newTable(rawName string) Table {
	return Table{
		RawName: rawName,
		Name:    camelCase(rawName),
	}
}

func (t Table) Write(b *bytes.Buffer, outputType OutputType) {
	if outputType == sbTable {
		b.WriteString("type ")
		b.WriteString(t.Name)
		b.WriteString("Table struct {\nsb.Table `db:\"")
		b.WriteString(t.RawName)
		b.WriteString("\"`\n")
	} else {
		b.WriteString("type ")
		b.WriteString(t.Name)
		b.WriteString(" struct {\n")
	}
	for _, c := range t.Columns {
		c.Write(b, outputType)
	}
	b.WriteString("}\n")
}

func (t Table) ToString(outputType OutputType) string {
	b := pool.Get().(*bytes.Buffer)
	b.Reset()
	t.Write(b, outputType)
	output := b.Bytes()
	pool.Put(b)

	o, err := format.Source(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		return bytesToString(output)
	}
	return bytesToString(o)
}

type Column struct {
	RawName  string
	Name     string
	RawType  string
	Type     string
	Unsigned bool
}

func columnTypeToGo(rawType string, unsigned bool) string {
	var t string

	switch rawType {
	case "int", "mediumint":
		t = "int32"
	case "tinyint":
		t = "int8"
	case "smallint":
		t = "int16"
	case "bigint":
		t = "int64"
	case "bool":
		return "bool"
	case "float":
		return "float32"
	case "double":
		return "float64"
	case "varchar", "char", "text", "tinytext", "mediumtext", "longtext":
		return "string"
	case "binary", "varbinary", "blob", "TinyBlob", "mediumblob", "longblob":
		return "[]byte"
	case "datetime":
		return "time.Time"
	}

	if unsigned {
		return "u" + t
	}
	return t
}

func title(s string) string {
	t, ok := transformer[s]
	if ok {
		return t
	}
	return strings.Title(s)
}

func camelCase(s string) string {
	if s == "" {
		return ""
	}

	parts := strings.Split(s, "_")
	if len(parts) == 1 { // 只有一个词，直接返回
		return title(s)
	}

	for i := 0; i < len(parts); i++ {
		parts[i] = title(parts[i])
	}
	return strings.Join(parts, "")
}

func newColumn(rawName, rawType string, unsigned bool) Column {
	return Column{
		RawName:  rawName,
		Name:     camelCase(rawName),
		RawType:  rawType,
		Type:     columnTypeToGo(rawType, unsigned),
		Unsigned: unsigned,
	}
}

func (c Column) Write(b *bytes.Buffer, outputType OutputType) {
	if outputType == sbTable {
		b.WriteString(c.Name)
		b.WriteString(" sb.Column `db:\"")
		b.WriteString(c.RawName)
		b.WriteString("\"`\n")
	} else {
		b.WriteString(c.Name)
		b.WriteByte(' ')
		b.WriteString(c.Type)
		b.WriteString(" `db:\"")
		b.WriteString(c.RawName)
		if outputType == sqlTableWithJSON {
			b.WriteString("\" json:\"")
			b.WriteString(c.RawName)
		}
		b.WriteString("\"`\n")
	}
}
