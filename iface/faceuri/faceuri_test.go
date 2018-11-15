package faceuri_test

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v2"

	"ndn-dpdk/dpdk/dpdktestenv"
	"ndn-dpdk/iface/faceuri"
)

func TestParse(t *testing.T) {
	assert, _ := dpdktestenv.MakeAR(t)

	tests := []struct {
		input  string
		ok     bool
		output string // "" indicates same as input
	}{
		{"badscheme://", false, ""},
		{"ether://02-02-02-02-02-02@net_pcap0:2", true, ""},
		{"ether://net_pcap0", true, "ether://01-00-5E-00-17-AA@net_pcap0:0"},
		{"ether://02-02-02-02-aa-bb@net_pcap0", true, "ether://02-02-02-02-AA-BB@net_pcap0:0"},
		{"ether://02-02-02-02-02-02@:2", false, ""},
		{"ether://02-02-02-02-02-02-02-02@net_pcap0:2", false, ""},
		{"ether://02-02-02-02-02-02@net_pcap0:-1", false, ""},
		{"ether://02-02-02-02-02-02@net_pcap0:4095", false, ""},
		{"ether://02-02-02-02-02-02:password@net_pcap0", false, ""},
		{"ether://net_pcap0/path", false, ""},
		{"ether://net_pcap0?query", false, ""},
		{"ether://net_pcap0#fragment", false, ""},
		{"mock:", true, ""},
		{"mock://x", false, ""},
		{"udp://192.0.2.1", true, "udp4://192.0.2.1:6363"},
		{"udp://192.0.2.1:7777", true, "udp4://192.0.2.1:7777"},
		{"udp4://192.0.2.1", true, "udp4://192.0.2.1:6363"},
		{"udp4://192.0.2.1:7777", true, ""},
		{"udp4://192.0.2.1/", true, "udp4://192.0.2.1:6363"},
		{"udp4://user@192.0.2.1", false, ""},
		{"udp4://0.0.0.0", false, ""},
		{"udp4://example.net", false, ""},
		{"udp4://255.255.255.255", false, ""},
		{"udp4://192.0.2.1:0", false, ""},
		{"udp4://192.0.2.1:77777", false, ""},
		{"udp4://192.0.2.1:dns", false, ""},
		{"udp4://192.0.2.1/path", false, ""},
		{"udp4://192.0.2.1?query", false, ""},
		{"udp4://192.0.2.1#fragment", false, ""},
		{"unix://", false, ""},
		{"unix:///", true, ""},
		{"unix:///var/run/ndn-dpdk-app.sock", true, ""},
		{"unix:///var//run/X/../ndn-dpdk-app.sock", true, "unix:///var/run/ndn-dpdk-app.sock"},
		{"tcp://192.0.2.1", true, "tcp4://192.0.2.1:6363"},
		{"tcp://192.0.2.1:7777", true, "tcp4://192.0.2.1:7777"},
		{"tcp4://192.0.2.1", true, "tcp4://192.0.2.1:6363"},
		{"tcp4://192.0.2.1:7777", true, ""},
	}
	for _, tt := range tests {
		u, e := faceuri.Parse(tt.input)
		if tt.ok {
			if assert.NoError(e, tt.input) && assert.NotNil(u, tt.input) {
				output := tt.output
				if output == "" {
					output = tt.input
				}
				assert.Equal(output, u.String())
			}
		} else {
			assert.Error(e, tt.input)
		}
	}
}

type marshalTestStruct struct {
	U *faceuri.FaceUri
	M faceuri.MacAddress
	I int
}

func TestMarshal(t *testing.T) {
	assert, _ := dpdktestenv.MakeAR(t)

	var obj marshalTestStruct
	obj.U = faceuri.MustParse("udp4://192.0.2.1:6363")
	obj.M, _ = faceuri.ParseMacAddress("02-02-02-AA-BB-CC")
	obj.I = 50

	jsonEncoding, e := json.Marshal(obj)
	if assert.NoError(e) {
		assert.Equal([]byte("{\"U\":\"udp4://192.0.2.1:6363\",\"M\":\"02-02-02-AA-BB-CC\",\"I\":50}"), jsonEncoding)
	}

	var jsonDecoded marshalTestStruct
	if e := json.Unmarshal(jsonEncoding, &jsonDecoded); assert.NoError(e) {
		assert.Equal("udp4://192.0.2.1:6363", jsonDecoded.U.String())
		assert.Equal(50, jsonDecoded.I)
	}

	var jsonDecoded2 marshalTestStruct
	assert.Error(json.Unmarshal([]byte("{\"U\":\"bad-scheme://\"}"), &jsonDecoded2))
	assert.Error(json.Unmarshal([]byte("{\"U\":4}"), &jsonDecoded2))
	assert.Error(json.Unmarshal([]byte("{\"M\":\"02-02-02-02-AA-BB-CC-DD\"}"), &jsonDecoded2))
	assert.Error(json.Unmarshal([]byte("{\"M\":4}"), &jsonDecoded2))
	if assert.NoError(json.Unmarshal([]byte("{\"U\":null,\"M\":null,\"I\":50}"), &jsonDecoded2)) {
		assert.Nil(jsonDecoded2.U)
		assert.Nil(jsonDecoded2.M)
	}

	yamlEncoding, e := yaml.Marshal(obj)
	if assert.NoError(e) {
		assert.Equal([]byte("u: udp4://192.0.2.1:6363\nm: 02-02-02-AA-BB-CC\ni: 50\n"), yamlEncoding)
	}

	var yamlDecoded marshalTestStruct
	if e := yaml.Unmarshal(yamlEncoding, &yamlDecoded); assert.NoError(e) {
		assert.Equal("udp4://192.0.2.1:6363", yamlDecoded.U.String())
		assert.Equal("02-02-02-AA-BB-CC", yamlDecoded.M.String())
		assert.Equal(50, yamlDecoded.I)
	}
}
