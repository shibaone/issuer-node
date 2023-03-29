package jsonschema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	core "github.com/iden3/go-iden3-core"
	jsonSuite "github.com/iden3/go-schema-processor/json"
	"github.com/iden3/go-schema-processor/processor"
	"github.com/iden3/go-schema-processor/utils"
	"github.com/mitchellh/mapstructure"

	"github.com/polygonid/sh-id-platform/internal/core/domain"
	"github.com/polygonid/sh-id-platform/internal/loader"
)

// ErrProcessSchema - something wrong happened when the schema loaded is processed
var ErrProcessSchema = errors.New("cannot process schema")

// Attributes is a list of Attribute entities
type Attributes []Attribute

// SchemaAttrs converts jsonschema.Attributes into domain.SchemaAttrs
func (a Attributes) SchemaAttrs() domain.SchemaAttrs {
	out := make(domain.SchemaAttrs, len(a))
	for i, attr := range a {
		out[i] = attr.String()
	}
	return out
}

// Attribute represents a json schema attribute
type Attribute struct {
	ID     string
	Title  string
	Type   string
	Format string
}

func (a Attribute) String() string {
	if a.Title != "" {
		return fmt.Sprintf("%s(%s)", a.Title, a.ID)
	}
	return a.ID
}

// JSONSchema provides some methods to load a schema and do some inspections over it.
type JSONSchema struct {
	content map[string]any
}

// Load loads the json file doing some validations..
func Load(ctx context.Context, loader loader.Loader) (*JSONSchema, error) {
	pr := processor.InitProcessorOptions(
		&processor.Processor{},
		processor.WithValidator(jsonSuite.Validator{}),
		processor.WithParser(jsonSuite.Parser{}),
		processor.WithSchemaLoader(loader))
	raw, _, err := pr.Load(ctx)
	if err != nil {
		return nil, err
	}

	schema := &JSONSchema{content: make(map[string]any)}
	if err := json.Unmarshal(raw, &schema.content); err != nil {
		return nil, err
	}
	return schema, nil
}

// AttributeNames returns a list with the attributes in properties.credentialSubject.properties
func (s *JSONSchema) AttributeNames() (Attributes, error) {
	var props map[string]any
	var ok bool
	props, ok = s.content["properties"].(map[string]any)
	if !ok {
		return nil, errors.New("missing properties field")
	}
	credSubject, ok := props["credentialSubject"].(map[string]any)
	if !ok {
		return nil, errors.New("missing properties.credentialSubject field")
	}
	props, ok = credSubject["properties"].(map[string]any)
	if !ok {
		return nil, errors.New("missing properties.credentialSubject.properties field")
	}
	attrs := make([]Attribute, 0, len(props))
	for id, prop := range props {
		attr := Attribute{}
		if err := mapstructure.Decode(prop, &attr); err != nil {
			return nil, fmt.Errorf("parsing attribute <%s>: %w", prop, err)
		}
		attr.ID = id
		attrs = append(attrs, attr)
	}
	return attrs, nil
}

// JSONLdContext returns the value of $metadata.uris.jsonLdContext
func (s *JSONSchema) JSONLdContext() (string, error) {
	var metadata map[string]any
	var ok bool
	metadata, ok = s.content["$metadata"].(map[string]any)
	if !ok {
		return "", errors.New("missing $metadata field")
	}
	uris, ok := metadata["uris"].(map[string]any)
	if !ok {
		return "", errors.New("missing $metadata.uris field")
	}
	jsonLdContext, ok := uris["jsonLdContext"].(string)
	if !ok {
		return "", errors.New("missing $metadata.uris.jsonLdContext field")
	}
	return jsonLdContext, nil
}

// SchemaHash calculates the hash of a schemaType
func (s *JSONSchema) SchemaHash(schemaType string) (core.SchemaHash, error) {
	jsonLdContext, err := s.JSONLdContext()
	if err != nil {
		return core.SchemaHash{}, err
	}
	id := jsonLdContext + "#" + schemaType
	return utils.CreateSchemaHash([]byte(id)), nil
}

// ValidateAndConvert - validates an array of attributes against the schema. Returns a new array with the credential attributes types converted.
func (s *JSONSchema) ValidateAndConvert(credentialAttributes []domain.CredentialAttributes) ([]domain.CredentialAttributes, error) {
	schemaAttributes, err := s.AttributeNames()
	if err != nil {
		return nil, ErrProcessSchema
	}
	for i, attributeLink := range credentialAttributes {
		attributeLinkName := attributeLink.Name
		attributeLinkValue := attributeLink.Value
		index := findIndexForSchemaAttribute(schemaAttributes, attributeLinkName)
		attributeLinkValueConverted, err := validateCredentialLinkAttribute(schemaAttributes[index], attributeLinkName, attributeLinkValue)
		if err != nil {
			return nil, err
		}
		credentialAttributes[i].Value = attributeLinkValueConverted
		schemaAttributes = removeIndex(schemaAttributes, index)
	}

	if len(schemaAttributes) != 1 {
		return nil, newCredentialLinkAttributeError("the number of attributes is not valid")
	}

	return credentialAttributes, nil
}

func findIndexForSchemaAttribute(attributes Attributes, name string) int {
	for i, attribute := range attributes {
		if attribute.ID == name {
			return i
		}
	}
	return -1
}

func validateCredentialLinkAttribute(schemaAttribute Attribute, attributeLinkName string, attributeLinkValue interface{}) (interface{}, error) {
	if schemaAttribute.Type == "string" {
		s, ok := attributeLinkValue.(string)
		if !ok {
			return nil, newCredentialLinkAttributeError(fmt.Sprintf("error converting the attribute: %s", attributeLinkName))
		}
		return s, nil
	}

	if schemaAttribute.Type == "integer" {
		s, ok := attributeLinkValue.(string)
		if !ok {
			return nil, newCredentialLinkAttributeError(fmt.Sprintf("error converting the attribute: %s", attributeLinkName))
		}
		newValue, err := strconv.Atoi(s)
		if err != nil {
			return nil, newCredentialLinkAttributeError(fmt.Sprintf("error converting the attribute: %s. Must be an integer", attributeLinkName))
		}
		return newValue, nil
	}

	if schemaAttribute.Type == "boolean" {
		s, ok := attributeLinkValue.(string)
		if !ok {
			return nil, newCredentialLinkAttributeError(fmt.Sprintf("error converting the attribute: %s", attributeLinkName))
		}
		newValue, err := strconv.ParseBool(s)
		if err != nil {
			return nil, newCredentialLinkAttributeError(fmt.Sprintf("error converting the attribute: %s. Must be a boolean", attributeLinkName))
		}
		return newValue, nil
	}

	return nil, newCredentialLinkAttributeError(fmt.Sprintf("error converting the attribute: %s. type not supported", attributeLinkName))
}

func removeIndex(s []Attribute, index int) []Attribute {
	return append(s[:index], s[index+1:]...)
}
