package gqlserver_test

import (
	"reflect"
	"testing"

	"github.com/graphql-go/graphql"
	"github.com/usnistgov/ndn-dpdk/core/gqlserver"
)

type bindTestA struct {
	NoTag       int
	Skip        int     `json:"-"`
	RequiredInt int     `json:"requiredInt" gqldesc:"This is a required integer field."`
	OptionalInt int     `json:"optionalInt,omitempty"`
	Bool        bool    `json:"bool"`
	Float       float64 `json:"float" gqldesc:"It's a floating point number."`
	String      string  `json:"string"`
	Slice       []int   `json:"slice"`
	Array       [2]int  `json:"array"`
}

type bindTestB struct {
	V *int `json:"v"`
}

func makeBindTestB(v int) (b bindTestB) {
	b.V = &v
	return b
}

type bindTestC struct {
	bindTestA
	RequiredB bindTestB  `json:"requiredB"`
	OptionalB *bindTestB `json:"optionalB"`
}

var gqlTypeB = graphql.NewObject(graphql.ObjectConfig{
	Name:   "B",
	Fields: gqlserver.BindFields[bindTestB](nil),
})

var bindTypesC = map[string]graphql.Type{
	"requiredInt": gqlserver.NonNullInt,
	"optionalInt": graphql.Int,
	"bool":        gqlserver.NonNullBoolean,
	"float":       graphql.NewNonNull(graphql.Float),
	"string":      gqlserver.NonNullString,
	"slice":       graphql.NewList(gqlserver.NonNullInt),
	"array":       gqlserver.NewListNonNullBoth(graphql.Int),
	"requiredB":   graphql.NewNonNull(gqlTypeB),
	"optionalB":   gqlTypeB,
}

func TestBindFields(t *testing.T) {
	assert, _ := makeAR(t)
	assert.Panics(func() { gqlserver.BindFields[bindTestC](nil) })

	fC := gqlserver.BindFields[bindTestC](gqlserver.FieldTypes{
		reflect.TypeFor[bindTestB](): gqlTypeB,
	})
	assert.Len(fC, len(bindTypesC))
	for fieldName, fieldType := range bindTypesC {
		assert.Equal(fieldType, fC[fieldName].Type, "%s", fieldName)
	}

	assert.Equal("This is a required integer field.", fC["requiredInt"].Description)
	assert.Equal("", fC["bool"].Description)
	assert.Equal("It's a floating point number.", fC["float"].Description)

	vC := bindTestC{
		bindTestA: bindTestA{
			RequiredInt: 10,
		},
		RequiredB: makeBindTestB(20),
		OptionalB: nil,
	}
	if v, e := fC["requiredInt"].Resolve(graphql.ResolveParams{Source: vC}); assert.NoError(e) {
		assert.Equal(10, v)
	}
	if v, e := fC["optionalInt"].Resolve(graphql.ResolveParams{Source: vC}); assert.NoError(e) {
		assert.Equal(0, v)
	}
	if v, e := fC["requiredB"].Resolve(graphql.ResolveParams{Source: vC}); assert.NoError(e) {
		if b, ok := v.(bindTestB); assert.True(ok) && assert.NotNil(b.V) {
			assert.Equal(20, *b.V)
		}
	}
	if v, e := fC["optionalB"].Resolve(graphql.ResolveParams{Source: vC}); assert.NoError(e) {
		assert.Nil(v)
	}

	vC.OptionalB = &bindTestB{}
	*vC.OptionalB = makeBindTestB(30)
	if v, e := fC["optionalB"].Resolve(graphql.ResolveParams{Source: vC}); assert.NoError(e) {
		if b, ok := v.(*bindTestB); assert.True(ok) && assert.NotNil(b.V) {
			assert.Equal(30, *b.V)
		}
	}
}

func TestBindInputFields(t *testing.T) {
	assert, _ := makeAR(t)
	assert.Panics(func() { gqlserver.BindInputFields[bindTestC](nil) })

	iC := gqlserver.BindInputFields[bindTestC](gqlserver.FieldTypes{
		reflect.TypeFor[bindTestB](): gqlTypeB,
	})
	assert.Len(iC, len(bindTypesC))
	for fieldName, fieldType := range bindTypesC {
		assert.Equal(fieldType, iC[fieldName].Type, "%s", fieldName)
	}
}
