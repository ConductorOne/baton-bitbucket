package field

import (
	"errors"
	"fmt"
	"strings"
)

var WrongValueTypeErr = errors.New("unable to cast any to concrete type")

type Variant string

const (
	StringVariant      Variant = "StringField"
	BoolVariant        Variant = "BoolField"
	IntVariant         Variant = "IntField"
	StringSliceVariant Variant = "StringSliceField"
)

type WebFieldType string

const (
	Randomize               WebFieldType = "RANDOMIZE"
	OAuth2                  WebFieldType = "OAUTH2"
	ConnectorDerivedOptions WebFieldType = "CONNECTOR_DERIVED_OPTIONS"
	FileUpload              WebFieldType = "FILE_UPLOAD"
)

type FieldRule struct {
	s *StringRules
	// e  *EnumRules
	ss *RepeatedRules[StringRules]
	b  *BoolRules
	i  *IntRules
}

// UIHints should be JSON??

type SyncerConfig struct {
	Required   bool
	Ignore     bool
	Hidden     bool
	ShortHand  string
	Persistent bool
}

type ConnectorConfig struct {
	DisplayName string
	Required    bool
	Ignore      bool
	Hidden      bool
	Placeholder string
	FieldType   WebFieldType
	// Only used by file uploads atm :-/.
	AllowedExtensions []string
}

type SchemaField struct {
	FieldName    string
	Required     bool
	DefaultValue any
	Description  string
	Ops          bool
	HelpURL      string

	Variant Variant
	Rules   FieldRule
	Secret  bool

	SyncerConfig    SyncerConfig
	ConnectorConfig ConnectorConfig
}

type SchemaTypes interface {
	~string | ~bool | ~int | ~uint | ~[]string
}

func (s SchemaField) GetName() string {
	return s.FieldName
}

func (s SchemaField) GetCLIShortHand() string {
	return s.SyncerConfig.ShortHand
}

func (s SchemaField) IsPersistent() bool {
	return s.SyncerConfig.Persistent
}

func (s SchemaField) IsHidden() bool {
	return s.SyncerConfig.Hidden
}

func (s SchemaField) GetDescription() string {
	var line string
	if s.Description == "" {
		line = fmt.Sprintf("($BATON_%s)", toUpperCase(s.FieldName))
	} else {
		line = fmt.Sprintf("%s ($BATON_%s)", s.Description, toUpperCase(s.FieldName))
	}

	if s.Required {
		line = fmt.Sprintf("required: %s", line)
	}

	return line
}

func (s SchemaField) Validate(value any) error {
	switch s.Variant {
	case StringVariant:
		v, ok := value.(string)
		if !ok {
			return WrongValueTypeErr
		}
		return s.Rules.s.Validate(v)
	case BoolVariant:
		v, ok := value.(bool)
		if !ok {
			return WrongValueTypeErr
		}
		return s.Rules.b.Validate(v)
	case IntVariant:
		v, ok := value.(int64)
		if !ok {
			return WrongValueTypeErr
		}
		return s.Rules.i.Validate(v)
	case StringSliceVariant:
		v, ok := value.([]string)
		if !ok {
			return WrongValueTypeErr
		}
		return s.Rules.ss.Validate(v)
	default:
		return fmt.Errorf("unknown field type %s", s.Variant)
	}
}

func toUpperCase(i string) string {
	return strings.ReplaceAll(strings.ToUpper(i), "-", "_")
}

// SchemaField can't be generic over SchemaTypes without breaking backwards compatibility :-/.
func GetDefaultValue[T SchemaTypes](s SchemaField) (*T, error) {
	value, ok := s.DefaultValue.(T)
	if !ok {
		return nil, WrongValueTypeErr
	}
	return &value, nil
}

func BoolField(name string, optional ...fieldOption) SchemaField {
	field := SchemaField{
		FieldName:       name,
		Variant:         BoolVariant,
		DefaultValue:    false,
		Rules:           FieldRule{}, // b: &BoolRules{},
		SyncerConfig:    SyncerConfig{},
		ConnectorConfig: ConnectorConfig{Ignore: true},
	}

	for _, o := range optional {
		field = o(field)
	}

	if field.Required {
		panic(fmt.Sprintf("requiring %s of type %s does not make sense", field.FieldName, field.Variant))
	}

	return field
}

func StringField(name string, optional ...fieldOption) SchemaField {
	field := SchemaField{
		FieldName:       name,
		Variant:         StringVariant,
		DefaultValue:    "",
		Rules:           FieldRule{}, // s: &StringRules{},
		SyncerConfig:    SyncerConfig{},
		ConnectorConfig: ConnectorConfig{Ignore: true},
	}

	for _, o := range optional {
		field = o(field)
	}

	return field
}

func IntField(name string, optional ...fieldOption) SchemaField {
	field := SchemaField{
		FieldName:       name,
		Variant:         IntVariant,
		DefaultValue:    0,
		Rules:           FieldRule{}, // i: &IntRules{},
		SyncerConfig:    SyncerConfig{},
		ConnectorConfig: ConnectorConfig{Ignore: true},
	}

	for _, o := range optional {
		field = o(field)
	}

	return field
}

func StringSliceField(name string, optional ...fieldOption) SchemaField {
	field := SchemaField{
		FieldName:       name,
		Variant:         StringSliceVariant,
		DefaultValue:    []string{},
		Rules:           FieldRule{}, // ss: &RepeatedRules[StringRules]{},
		SyncerConfig:    SyncerConfig{},
		ConnectorConfig: ConnectorConfig{Ignore: true},
	}

	for _, o := range optional {
		field = o(field)
	}

	return field
}

func SelectField(name string, options []string, optional ...fieldOption) SchemaField {
	field := SchemaField{
		FieldName:    name,
		Variant:      StringVariant,
		DefaultValue: "",
		Rules: FieldRule{
			s: &StringRules{In: options},
		},
		SyncerConfig:    SyncerConfig{},
		ConnectorConfig: ConnectorConfig{Ignore: true},
	}

	for _, o := range optional {
		field = o(field)
	}

	return field
}
