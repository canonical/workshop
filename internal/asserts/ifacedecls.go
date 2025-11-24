package asserts

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	defaultOutcome = map[string]interface{}{
		"allow-installation":    "true",
		"allow-connection":      "true",
		"allow-auto-connection": "true",
		"deny-installation":     "false",
		"deny-connection":       "false",
		"deny-auto-connection":  "false",
	}

	invertedOutcome = map[string]interface{}{
		"allow-installation":    "false",
		"allow-connection":      "false",
		"allow-auto-connection": "false",
		"deny-installation":     "true",
		"deny-connection":       "true",
		"deny-auto-connection":  "true",
	}

	ruleSubrules = []string{"allow-installation", "deny-installation", "allow-connection", "deny-connection", "allow-auto-connection", "deny-auto-connection"}

	validSdkType = regexp.MustCompile(`^(?:system|regular)$`)

	validIDConstraints = map[string]*regexp.Regexp{
		"slot-sdk-type": validSdkType,
		"plug-sdk-type": validSdkType,
	}

	nameConstraints      = []string{"plug-names", "slot-names"}
	attributeConstraints = []string{"plug-attributes", "slot-attributes"}
	idConstraints        = []string{"plug-sdk-type", "slot-sdk-type"}

	sideArityConstraints        = []string{"slots-per-plug", "plugs-per-slot"}
	sideArityConstraintsSetters = map[string]func(sideArityConstraintsHolder, SideArityConstraint){
		"slots-per-plug": sideArityConstraintsHolder.setSlotsPerPlug,
		"plugs-per-slot": sideArityConstraintsHolder.setPlugsPerSlot,
	}
)

const (
	// feature label for plug-names/slot-names constraints
	nameConstraintsFeature = "name-constraints"
)

// SlotInstallationConstraints specifies a set of constraints on an
// interface slot relevant to the installation of SDK.
type SlotInstallationConstraints struct {
	SlotSdkTypes   []string
	SlotAttributes *AttributeConstraints
	SlotNames      *NameConstraints
}

func (c *SlotInstallationConstraints) feature(flabel string) bool {
	if flabel == nameConstraintsFeature {
		return c.SlotNames != nil
	}
	return c.SlotAttributes.feature(flabel)
}

func (c *SlotInstallationConstraints) setNameConstraints(field string, cstrs *NameConstraints) {
	switch field {
	case "slot-names":
		c.SlotNames = cstrs
	default:
		panic("unknown SlotInstallationConstraints field " + field)
	}
}

func (c *SlotInstallationConstraints) setIDConstraints(field string, cstrs []string) {
	switch field {
	case "slot-sdk-type":
		c.SlotSdkTypes = cstrs
	default:
		panic("unknown SlotInstallationConstraints field " + field)
	}
}

func (c *SlotInstallationConstraints) setAttributeConstraints(field string, cstrs *AttributeConstraints) {
	switch field {
	case "slot-attributes":
		c.SlotAttributes = cstrs
	default:
		panic("unknown SlotInstallationConstraints field " + field)
	}
}

// SideArityConstraint specifies a constraint for the overall arity of
// the set of connected slots for a given plug or the set of
// connected plugs for a given slot.
// It is used to express parsed slots-per-plug and plugs-per-slot
// constraints.
// See https://forum.snapcraft.io/t/plug-slot-declaration-rules-greedy-plugs/12438
type SideArityConstraint struct {
	// N can be:
	// =>1
	// 0 means default and is used only internally during rule
	// compilation or on deny- rules where these constraints are
	// not applicable
	// -1 represents *, that means any (number of)
	N int
}

// Any returns whether this represents the * (any number of) constraint.
func (ac SideArityConstraint) Any() bool {
	return ac.N == -1
}

// SlotConnectionConstraints specfies a set of constraints on an
// interface slot for a sdk relevant to its connection or
// auto-connection.
type SlotConnectionConstraints struct {
	SlotSdkTypes []string
	PlugSdkTypes []string

	SlotNames *NameConstraints
	PlugNames *NameConstraints

	SlotAttributes *AttributeConstraints
	PlugAttributes *AttributeConstraints

	// SlotsPerPlug defaults to 1 for auto-connection, can be * (any)
	SlotsPerPlug SideArityConstraint
	// PlugsPerSlot is always * (any) (for now)
	PlugsPerSlot SideArityConstraint
}

func (c *SlotConnectionConstraints) feature(flabel string) bool {
	if flabel == nameConstraintsFeature {
		return c.PlugNames != nil || c.SlotNames != nil
	}
	return c.PlugAttributes.feature(flabel) || c.SlotAttributes.feature(flabel)
}

func (c *SlotConnectionConstraints) setNameConstraints(field string, cstrs *NameConstraints) {
	switch field {
	case "plug-names":
		c.PlugNames = cstrs
	case "slot-names":
		c.SlotNames = cstrs
	default:
		panic("unknown SlotConnectionConstraints field " + field)
	}
}

func (c *SlotConnectionConstraints) setIDConstraints(field string, cstrs []string) {
	switch field {
	case "plug-sdk-type":
		c.PlugSdkTypes = cstrs
	case "slot-sdk-type":
		c.SlotSdkTypes = cstrs
	default:
		panic("unknown SlotConnectionConstraints field " + field)
	}
}

func (c *SlotConnectionConstraints) setAttributeConstraints(field string, cstrs *AttributeConstraints) {
	switch field {
	case "plug-attributes":
		c.PlugAttributes = cstrs
	case "slot-attributes":
		c.SlotAttributes = cstrs
	default:
		panic("unknown SlotConnectionConstraints field " + field)
	}
}

func (c *SlotConnectionConstraints) setSlotsPerPlug(a SideArityConstraint) {
	c.SlotsPerPlug = a
}

func (c *SlotConnectionConstraints) setPlugsPerSlot(a SideArityConstraint) {
	c.PlugsPerSlot = a
}

func (c *SlotConnectionConstraints) slotsPerPlug() SideArityConstraint {
	return c.SlotsPerPlug
}

func (c *SlotConnectionConstraints) plugsPerSlot() SideArityConstraint {
	return c.PlugsPerSlot
}

type sideArityConstraintsHolder interface {
	setSlotsPerPlug(SideArityConstraint)
	setPlugsPerSlot(SideArityConstraint)

	slotsPerPlug() SideArityConstraint
	plugsPerSlot() SideArityConstraint
}

func normalizeSideArityConstraints(context *subruleContext, c sideArityConstraintsHolder) {
	if !context.allow() {
		return
	}
	any := SideArityConstraint{N: -1}
	// normalized plugs-per-slot is always *
	c.setPlugsPerSlot(any)
	slotsPerPlug := c.slotsPerPlug()
	if context.autoConnection() {
		// auto-connection slots-per-plug can be any or 1
		if !slotsPerPlug.Any() {
			c.setSlotsPerPlug(SideArityConstraint{N: 1})
		}
	} else {
		// connection slots-per-plug can be only any
		c.setSlotsPerPlug(any)
	}
}

type nameMatcher interface {
	match(name string, special map[string]string) error
}

// validates special name constraints like $INTERFACE
var validSpecialNameConstraint = regexp.MustCompile(`^\$[A-Z][A-Z0-9_]*$`)

func compileNameMatcher(whichName string, v interface{}) (nameMatcher, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("%s constraint entry must be a regexp or special $ value", whichName)
	}
	if strings.HasPrefix(s, "$") {
		if !validSpecialNameConstraint.MatchString(s) {
			return nil, fmt.Errorf("%s constraint entry special value %q is invalid", whichName, s)
		}
		return specialNameMatcher{special: s}, nil
	}
	if strings.IndexFunc(s, unicode.IsSpace) != -1 {
		return nil, fmt.Errorf("%s constraint entry regexp contains unexpected spaces", whichName)
	}
	rx, err := regexp.Compile("^(" + s + ")$")
	if err != nil {
		return nil, fmt.Errorf("cannot compile %s constraint entry %q: %v", whichName, s, err)
	}
	return regexpNameMatcher{rx}, nil
}

type regexpNameMatcher struct {
	*regexp.Regexp
}

func (matcher regexpNameMatcher) match(name string, special map[string]string) error {
	if !matcher.MatchString(name) {
		return fmt.Errorf("%q does not match %v", name, matcher.Regexp)
	}
	return nil
}

type specialNameMatcher struct {
	special string
}

func (matcher specialNameMatcher) match(name string, special map[string]string) error {
	expected := special[matcher.special]
	if expected == "" || expected != name {
		return fmt.Errorf("%q does not match %v", name, matcher.special)
	}
	return nil
}

// NameConstraints implements a set of constraints on the names of slots or plugs.
// See https://forum.snapcraft.io/t/plug-slot-rules-plug-names-slot-names-constraints/12439
type NameConstraints struct {
	matchers []nameMatcher
}

func compileNameConstraints(whichName string, constraints interface{}) (*NameConstraints, error) {
	l, ok := constraints.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s constraints must be a list of regexps and special $ values", whichName)
	}
	matchers := make([]nameMatcher, 0, len(l))
	for _, nm := range l {
		matcher, err := compileNameMatcher(whichName, nm)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, matcher)
	}
	return &NameConstraints{matchers: matchers}, nil
}

// Check checks whether name doesn't match the constraints.
func (nc *NameConstraints) Check(whichName, name string, special map[string]string) error {
	for _, m := range nc.matchers {
		if err := m.match(name, special); err == nil {
			return nil
		}
	}
	return fmt.Errorf("%s %q does not match constraints", whichName, name)
}

// SlotRule holds the rule of what is allowed, wrt installation and
// connection, for a slot of a specific interface for a SDK.
type SlotRule struct {
	Interface string

	AllowInstallation []*SlotInstallationConstraints
	DenyInstallation  []*SlotInstallationConstraints

	AllowConnection []*SlotConnectionConstraints
	DenyConnection  []*SlotConnectionConstraints

	AllowAutoConnection []*SlotConnectionConstraints
	DenyAutoConnection  []*SlotConnectionConstraints
}

func (r *SlotRule) setConstraints(field string, cstrs []constraintsHolder) {
	if len(cstrs) == 0 {
		panic(fmt.Sprintf("cannot set SlotRule field %q to empty", field))
	}
	switch cstrs[0].(type) {
	case *SlotInstallationConstraints:
		switch field {
		case "allow-installation":
			r.AllowInstallation = castSlotInstallationConstraints(cstrs)
			return
		case "deny-installation":
			r.DenyInstallation = castSlotInstallationConstraints(cstrs)
			return
		}
	case *SlotConnectionConstraints:
		switch field {
		case "allow-connection":
			r.AllowConnection = castSlotConnectionConstraints(cstrs)
			return
		case "deny-connection":
			r.DenyConnection = castSlotConnectionConstraints(cstrs)
			return
		case "allow-auto-connection":
			r.AllowAutoConnection = castSlotConnectionConstraints(cstrs)
			return
		case "deny-auto-connection":
			r.DenyAutoConnection = castSlotConnectionConstraints(cstrs)
			return
		}
	}
	panic(fmt.Sprintf("cannot set SlotRule field %q with %T elements", field, cstrs[0]))
}

func castSlotConnectionConstraints(cstrs []constraintsHolder) (res []*SlotConnectionConstraints) {
	res = make([]*SlotConnectionConstraints, len(cstrs))
	for i, cstr := range cstrs {
		res[i] = cstr.(*SlotConnectionConstraints)
	}
	return res
}

func castSlotInstallationConstraints(cstrs []constraintsHolder) (res []*SlotInstallationConstraints) {
	res = make([]*SlotInstallationConstraints, len(cstrs))
	for i, cstr := range cstrs {
		res[i] = cstr.(*SlotInstallationConstraints)
	}
	return res
}

func compileSlotInstallationConstraints(context *subruleContext, cDef constraintsDef) (constraintsHolder, error) {
	slotInstCstrs := &SlotInstallationConstraints{}
	err := baseCompileConstraints(context, cDef, slotInstCstrs, []string{"slot-names"}, []string{"slot-attributes"}, []string{"slot-sdk-type"})
	if err != nil {
		return nil, err
	}
	return slotInstCstrs, nil
}

func compileSlotConnectionConstraints(context *subruleContext, cDef constraintsDef) (constraintsHolder, error) {
	slotConnCstrs := &SlotConnectionConstraints{}
	err := baseCompileConstraints(context, cDef, slotConnCstrs, nameConstraints, attributeConstraints, idConstraints)
	if err != nil {
		return nil, err
	}
	normalizeSideArityConstraints(context, slotConnCstrs)
	return slotConnCstrs, nil
}

func compileSideArityConstraint(context *subruleContext, which string, v interface{}) (SideArityConstraint, error) {
	var a SideArityConstraint
	if context.installation() || !context.allow() {
		return a, fmt.Errorf("%v cannot specify a %v constraint, they apply only to allow-*connection", context, which)
	}
	x, ok := v.(string)
	if !ok || len(x) == 0 {
		return a, fmt.Errorf("%v in %v must be an integer >=1 or *", which, context)
	}
	if x == "*" {
		return SideArityConstraint{N: -1}, nil
	}
	n, err := atoi(x, "%v in %v", which, context)
	switch _, syntax := err.(intSyntaxError); {
	case err == nil && n < 1:
		fallthrough
	case syntax:
		return a, fmt.Errorf("%v in %v must be an integer >=1 or *", which, context)
	case err != nil:
		return a, err
	}
	return SideArityConstraint{N: n}, nil
}

type fixedAttrMatcher struct {
	result error
}

func (matcher fixedAttrMatcher) feature(flabel string) bool {
	return false
}

func (matcher fixedAttrMatcher) match(apath string, v interface{}, ctx *attrMatchingContext) error {
	return matcher.result
}

// AttrMatchContext has contextual helpers for evaluating attribute constraints.
type AttrMatchContext interface {
	PlugAttr(arg string) (interface{}, error)
	SlotAttr(arg string) (interface{}, error)
}

// AttributeConstraints implements a set of constraints on the attributes of a slot or plug.
type AttributeConstraints struct {
	matcher attrMatcher
}

func (ac *AttributeConstraints) feature(flabel string) bool {
	return ac.matcher.feature(flabel)
}

// Check checks whether attrs don't match the constraints.
func (c *AttributeConstraints) Check(attrer Attrer, helper AttrMatchContext) error {
	return c.matcher.match("", attrer, &attrMatchingContext{
		attrWord: "attribute",
		helper:   helper,
	})
}

// compileAttributeConstraints checks and compiles a mapping or list
// from the assertion format into AttributeConstraints.
func compileAttributeConstraints(constraints interface{}) (*AttributeConstraints, error) {
	cc := compileContext{
		opts: &compileAttrMatcherOptions{
			allowedOperations: []string{"SLOT", "PLUG"},
		},
	}
	matcher, err := compileAttrMatcher(cc, constraints)
	if err != nil {
		return nil, err
	}
	return &AttributeConstraints{matcher: matcher}, nil
}

var (
	AlwaysMatchAttributes = &AttributeConstraints{matcher: fixedAttrMatcher{nil}}
	NeverMatchAttributes  = &AttributeConstraints{matcher: fixedAttrMatcher{errors.New("not allowed")}}
)

// Attrer reflects part of the Attrer interface (see interfaces.Attrer).
type Attrer interface {
	Lookup(path string) (interface{}, bool)
}

func baseCompileConstraints(context *subruleContext, cDef constraintsDef, target constraintsHolder, nameConstraints, attrConstraints, idConstraints []string) error {
	cMap := cDef.cMap
	if cMap == nil {
		fixed := AlwaysMatchAttributes // "true"
		if cDef.invert {               // "false"
			fixed = NeverMatchAttributes
		}
		for _, field := range attrConstraints {
			target.setAttributeConstraints(field, fixed)
		}
		return nil
	}
	defaultUsed := 0
	for _, field := range nameConstraints {
		v := cMap[field]
		if v != nil {
			nc, err := compileNameConstraints(field, v)
			if err != nil {
				return err
			}
			target.setNameConstraints(field, nc)
		} else {
			defaultUsed++
		}
	}
	for _, field := range attrConstraints {
		cstrs := AlwaysMatchAttributes
		v := cMap[field]
		if v != nil {
			var err error
			cstrs, err = compileAttributeConstraints(cMap[field])
			if err != nil {
				return fmt.Errorf("cannot compile %s in %s: %v", field, context, err)
			}
		} else {
			defaultUsed++
		}
		target.setAttributeConstraints(field, cstrs)
	}
	for _, field := range idConstraints {
		lst, err := checkStringListInMap(cMap, field, fmt.Sprintf("%s in %v", field, context), validIDConstraints[field])
		if err != nil {
			return err
		}
		if lst == nil {
			defaultUsed++
		}
		target.setIDConstraints(field, lst)
	}
	for _, field := range sideArityConstraints {
		v := cMap[field]
		if v != nil {
			c, err := compileSideArityConstraint(context, field, v)
			if err != nil {
				return err
			}
			h, ok := target.(sideArityConstraintsHolder)
			if !ok {
				return fmt.Errorf("internal error: side arity constraint compiled for unexpected subrule %T", target)
			}
			sideArityConstraintsSetters[field](h, c)
		} else {
			defaultUsed++
		}
	}
	if defaultUsed == len(nameConstraints)+len(attributeConstraints)+len(idConstraints)+len(sideArityConstraints) {
		return fmt.Errorf("%s must specify at least one of %s, %s, %s, %s", context, strings.Join(nameConstraints, ", "), strings.Join(attrConstraints, ", "), strings.Join(idConstraints, ", "), strings.Join(sideArityConstraints, ", "))
	}
	return nil
}

// PlugInstallationConstraints specifies a set of constraints on an interface plug relevant to the installation of snap.
type PlugInstallationConstraints struct {
	PlugSdkTypes []string

	PlugNames *NameConstraints

	PlugAttributes *AttributeConstraints
}

func (c *PlugInstallationConstraints) feature(flabel string) bool {
	if flabel == nameConstraintsFeature {
		return c.PlugNames != nil
	}
	return c.PlugAttributes.feature(flabel)
}

func (c *PlugInstallationConstraints) setNameConstraints(field string, cstrs *NameConstraints) {
	switch field {
	case "plug-names":
		c.PlugNames = cstrs
	default:
		panic("unknown PlugInstallationConstraints field " + field)
	}
}

func (c *PlugInstallationConstraints) setAttributeConstraints(field string, cstrs *AttributeConstraints) {
	switch field {
	case "plug-attributes":
		c.PlugAttributes = cstrs
	default:
		panic("unknown PlugInstallationConstraints field " + field)
	}
}

func (c *PlugInstallationConstraints) setIDConstraints(field string, cstrs []string) {
	switch field {
	case "plug-sdk-type":
		c.PlugSdkTypes = cstrs
	default:
		panic("unknown PlugInstallationConstraints field " + field)
	}
}

func compilePlugInstallationConstraints(context *subruleContext, cDef constraintsDef) (constraintsHolder, error) {
	plugInstCstrs := &PlugInstallationConstraints{}
	// plug-snap-id is supported here mainly for symmetry with the slot case
	// see discussion there
	err := baseCompileConstraints(context, cDef, plugInstCstrs, []string{"plug-names"}, []string{"plug-attributes"}, []string{"plug-sdk-type"})
	if err != nil {
		return nil, err
	}
	return plugInstCstrs, nil
}

// PlugConnectionConstraints specfies a set of constraints on an
// interface plug for a snap relevant to its connection or
// auto-connection.
type PlugConnectionConstraints struct {
	PlugSdkTypes []string
	SlotSdkTypes []string

	PlugNames *NameConstraints
	SlotNames *NameConstraints

	PlugAttributes *AttributeConstraints
	SlotAttributes *AttributeConstraints

	// SlotsPerPlug defaults to 1 for auto-connection, can be * (any)
	SlotsPerPlug SideArityConstraint
	// PlugsPerSlot is always * (any) (for now)
	PlugsPerSlot SideArityConstraint
}

func (c *PlugConnectionConstraints) feature(flabel string) bool {
	if flabel == nameConstraintsFeature {
		return c.PlugNames != nil || c.SlotNames != nil
	}
	return c.PlugAttributes.feature(flabel) || c.SlotAttributes.feature(flabel)
}

func (c *PlugConnectionConstraints) setNameConstraints(field string, cstrs *NameConstraints) {
	switch field {
	case "plug-names":
		c.PlugNames = cstrs
	case "slot-names":
		c.SlotNames = cstrs
	default:
		panic("unknown PlugConnectionConstraints field " + field)
	}
}

func (c *PlugConnectionConstraints) setAttributeConstraints(field string, cstrs *AttributeConstraints) {
	switch field {
	case "plug-attributes":
		c.PlugAttributes = cstrs
	case "slot-attributes":
		c.SlotAttributes = cstrs
	default:
		panic("unknown PlugConnectionConstraints field " + field)
	}
}

func (c *PlugConnectionConstraints) setIDConstraints(field string, cstrs []string) {
	switch field {
	case "plug-sdk-type":
		c.PlugSdkTypes = cstrs
	case "slot-sdk-type":
		c.SlotSdkTypes = cstrs
	default:
		panic("unknown PlugConnectionConstraints field " + field)
	}
}

func (c *PlugConnectionConstraints) setSlotsPerPlug(a SideArityConstraint) {
	c.SlotsPerPlug = a
}

func (c *PlugConnectionConstraints) setPlugsPerSlot(a SideArityConstraint) {
	c.PlugsPerSlot = a
}

func (c *PlugConnectionConstraints) slotsPerPlug() SideArityConstraint {
	return c.SlotsPerPlug
}

func (c *PlugConnectionConstraints) plugsPerSlot() SideArityConstraint {
	return c.PlugsPerSlot
}

func compilePlugConnectionConstraints(context *subruleContext, cDef constraintsDef) (constraintsHolder, error) {
	plugConnCstrs := &PlugConnectionConstraints{}
	err := baseCompileConstraints(context, cDef, plugConnCstrs, nameConstraints, attributeConstraints, idConstraints)
	if err != nil {
		return nil, err
	}
	normalizeSideArityConstraints(context, plugConnCstrs)
	return plugConnCstrs, nil
}

// PlugRule holds the rule of what is allowed, wrt instalation and
// connection, for a plug of a specific interface for a sdk.
type PlugRule struct {
	Interface string

	AllowInstallation []*PlugInstallationConstraints
	DenyInstallation  []*PlugInstallationConstraints

	AllowConnection []*PlugConnectionConstraints
	DenyConnection  []*PlugConnectionConstraints

	AllowAutoConnection []*PlugConnectionConstraints
	DenyAutoConnection  []*PlugConnectionConstraints
}

func (r *PlugRule) feature(flabel string) bool {
	for _, cs := range [][]*PlugInstallationConstraints{r.AllowInstallation, r.DenyInstallation} {
		for _, c := range cs {
			if c.feature(flabel) {
				return true
			}
		}
	}

	for _, cs := range [][]*PlugConnectionConstraints{r.AllowConnection, r.DenyConnection, r.AllowAutoConnection, r.DenyAutoConnection} {
		for _, c := range cs {
			if c.feature(flabel) {
				return true
			}
		}
	}

	return false
}

func castPlugInstallationConstraints(cstrs []constraintsHolder) (res []*PlugInstallationConstraints) {
	res = make([]*PlugInstallationConstraints, len(cstrs))
	for i, cstr := range cstrs {
		res[i] = cstr.(*PlugInstallationConstraints)
	}
	return res
}

func castPlugConnectionConstraints(cstrs []constraintsHolder) (res []*PlugConnectionConstraints) {
	res = make([]*PlugConnectionConstraints, len(cstrs))
	for i, cstr := range cstrs {
		res[i] = cstr.(*PlugConnectionConstraints)
	}
	return res
}

func (r *PlugRule) setConstraints(field string, cstrs []constraintsHolder) {
	if len(cstrs) == 0 {
		panic(fmt.Sprintf("cannot set PlugRule field %q to empty", field))
	}
	switch cstrs[0].(type) {
	case *PlugInstallationConstraints:
		switch field {
		case "allow-installation":
			r.AllowInstallation = castPlugInstallationConstraints(cstrs)
			return
		case "deny-installation":
			r.DenyInstallation = castPlugInstallationConstraints(cstrs)
			return
		}
	case *PlugConnectionConstraints:
		switch field {
		case "allow-connection":
			r.AllowConnection = castPlugConnectionConstraints(cstrs)
			return
		case "deny-connection":
			r.DenyConnection = castPlugConnectionConstraints(cstrs)
			return
		case "allow-auto-connection":
			r.AllowAutoConnection = castPlugConnectionConstraints(cstrs)
			return
		case "deny-auto-connection":
			r.DenyAutoConnection = castPlugConnectionConstraints(cstrs)
			return
		}
	}
	panic(fmt.Sprintf("cannot set PlugRule field %q with %T elements", field, cstrs[0]))
}

var plugRuleCompilers = map[string]subruleCompiler{
	"allow-installation":    compilePlugInstallationConstraints,
	"deny-installation":     compilePlugInstallationConstraints,
	"allow-connection":      compilePlugConnectionConstraints,
	"deny-connection":       compilePlugConnectionConstraints,
	"allow-auto-connection": compilePlugConnectionConstraints,
	"deny-auto-connection":  compilePlugConnectionConstraints,
}

func compilePlugRule(interfaceName string, rule interface{}) (*PlugRule, error) {
	context := fmt.Sprintf("plug rule for interface %q", interfaceName)
	plugRule := &PlugRule{
		Interface: interfaceName,
	}
	err := baseCompileRule(context, rule, plugRule, ruleSubrules, plugRuleCompilers, defaultOutcome, invertedOutcome)
	if err != nil {
		return nil, err
	}
	return plugRule, nil
}

// subruleContext carries queryable context information about one the
// {allow,deny}-* subrules that end up compiled as
// Plug|Slot*Constraints.  The information includes the parent rule,
// the introductory subrule key ({allow,deny}-*) and which alternative
// it corresponds to if any.
// The information is useful for constraints compilation now that we
// have constraints with different behavior depending on the kind of
// subrule that hosts them (e.g. slots-per-plug, plugs-per-slot).
type subruleContext struct {
	// rule is the parent rule context description
	rule string
	// subrule is the subrule key
	subrule string
	// alt is which alternative this is (if > 0)
	alt int
}

func (c *subruleContext) String() string {
	subctxt := fmt.Sprintf("%s in %s", c.subrule, c.rule)
	if c.alt != 0 {
		subctxt = fmt.Sprintf("alternative %d of %s", c.alt, subctxt)
	}
	return subctxt
}

// allow returns whether the subrule is an allow-* subrule.
func (c *subruleContext) allow() bool {
	return strings.HasPrefix(c.subrule, "allow-")
}

// installation returns whether the subrule is an *-installation subrule.
func (c *subruleContext) installation() bool {
	return strings.HasSuffix(c.subrule, "-installation")
}

// autoConnection returns whether the subrule is an *-auto-connection subrule.
func (c *subruleContext) autoConnection() bool {
	return strings.HasSuffix(c.subrule, "-auto-connection")
}

type constraintsDef struct {
	cMap   map[string]interface{}
	invert bool
}

type constraintsHolder interface {
	setNameConstraints(field string, cstrs *NameConstraints)
	setIDConstraints(field string, cstrs []string)
	setAttributeConstraints(field string, cstrs *AttributeConstraints)
}

type rule interface {
	setConstraints(field string, cstrs []constraintsHolder)
}

type subruleCompiler func(context *subruleContext, def constraintsDef) (constraintsHolder, error)

var slotRuleCompilers = map[string]subruleCompiler{
	"allow-installation":    compileSlotInstallationConstraints,
	"deny-installation":     compileSlotInstallationConstraints,
	"allow-connection":      compileSlotConnectionConstraints,
	"deny-connection":       compileSlotConnectionConstraints,
	"allow-auto-connection": compileSlotConnectionConstraints,
	"deny-auto-connection":  compileSlotConnectionConstraints,
}

func compileSlotRule(interfaceName string, rule interface{}) (*SlotRule, error) {
	context := fmt.Sprintf("slot rule for interface %q", interfaceName)
	slotRule := &SlotRule{
		Interface: interfaceName,
	}
	err := baseCompileRule(context, rule, slotRule, ruleSubrules, slotRuleCompilers, defaultOutcome, invertedOutcome)
	if err != nil {
		return nil, err
	}
	return slotRule, nil
}

func checkMapOrShortcut(v interface{}) (m map[string]interface{}, invert bool, err error) {
	switch x := v.(type) {
	case map[string]interface{}:
		return x, false, nil
	case string:
		switch x {
		case "true":
			return nil, false, nil
		case "false":
			return nil, true, nil
		}
	}
	return nil, false, errors.New("unexpected type")
}

func baseCompileRule(context string, rule interface{}, target rule, subrules []string, compilers map[string]subruleCompiler, defaultOutcome, invertedOutcome map[string]interface{}) error {
	rMap, invert, err := checkMapOrShortcut(rule)
	if err != nil {
		return fmt.Errorf("%s must be a map or one of the shortcuts 'true' or 'false'", context)
	}
	if rMap == nil {
		rMap = defaultOutcome // "true"
		if invert {
			rMap = invertedOutcome // "false"
		}
	}
	defaultUsed := 0
	// compile and set subrules
	for _, subrule := range subrules {
		v := rMap[subrule]
		var lst []interface{}
		alternatives := false
		switch x := v.(type) {
		case nil:
			v = defaultOutcome[subrule]
			defaultUsed++
		case []interface{}:
			alternatives = true
			lst = x
		}
		if lst == nil { // v is map or a string, checked below
			lst = []interface{}{v}
		}
		compiler := compilers[subrule]
		if compiler == nil {
			panic(fmt.Sprintf("no compiler for %s in %s", subrule, context))
		}
		alts := make([]constraintsHolder, len(lst))
		for i, alt := range lst {
			subctxt := &subruleContext{
				rule:    context,
				subrule: subrule,
			}
			if alternatives {
				subctxt.alt = i + 1
			}
			cMap, invert, err := checkMapOrShortcut(alt)
			if err != nil || (cMap == nil && alternatives) {
				efmt := "%s must be a map"
				if !alternatives {
					efmt = "%s must be a map or one of the shortcuts 'true' or 'false'"
				}
				return fmt.Errorf(efmt, subctxt)
			}

			cstrs, err := compiler(subctxt, constraintsDef{
				cMap:   cMap,
				invert: invert,
			})
			if err != nil {
				return err
			}
			alts[i] = cstrs
		}
		target.setConstraints(subrule, alts)
	}
	if defaultUsed == len(subrules) {
		return fmt.Errorf("%s must specify at least one of %s", context, strings.Join(subrules, ", "))
	}
	return nil
}
