package osiris

import (
	"math"
	"math/big"
)

type namedValueFactory func() Value

var namedConstantFactories = map[string]namedValueFactory{
	"Meter":    unitFactory("Meter"),
	"meter":    unitFactory("Meter"),
	"m":        unitFactory("Meter"),
	"Second":   unitFactory("Second"),
	"second":   unitFactory("Second"),
	"s":        unitFactory("Second"),
	"Kilogram": unitFactory("Kilogram"),
	"kilogram": unitFactory("Kilogram"),
	"kg":       unitFactory("Kilogram"),
	"Ampere":   unitFactory("Ampere"),
	"ampere":   unitFactory("Ampere"),
	"A":        unitFactory("Ampere"),
	"Kelvin":   unitFactory("Kelvin"),
	"kelvin":   unitFactory("Kelvin"),
	"K":        unitFactory("Kelvin"),
	"Mole":     unitFactory("Mole"),
	"mole":     unitFactory("Mole"),
	"mol":      unitFactory("Mole"),
	"Candela":  unitFactory("Candela"),
	"candela":  unitFactory("Candela"),
	"cd":       unitFactory("Candela"),

	"Hertz":     quantityFactory(1, map[string]int{"Second": -1}),
	"hertz":     quantityFactory(1, map[string]int{"Second": -1}),
	"Hz":        quantityFactory(1, map[string]int{"Second": -1}),
	"Newton":    quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 1, "Second": -2}),
	"newton":    quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 1, "Second": -2}),
	"N":         quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 1, "Second": -2}),
	"Pascal":    quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": -1, "Second": -2}),
	"pascal":    quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": -1, "Second": -2}),
	"Pa":        quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": -1, "Second": -2}),
	"Joule":     quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2}),
	"joule":     quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2}),
	"J":         quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2}),
	"Watt":      quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -3}),
	"watt":      quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -3}),
	"W":         quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -3}),
	"Coulomb":   quantityFactory(1, map[string]int{"Ampere": 1, "Second": 1}),
	"coulomb":   quantityFactory(1, map[string]int{"Ampere": 1, "Second": 1}),
	"C":         quantityFactory(1, map[string]int{"Ampere": 1, "Second": 1}),
	"Volt":      quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -3, "Ampere": -1}),
	"volt":      quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -3, "Ampere": -1}),
	"V":         quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -3, "Ampere": -1}),
	"Farad":     quantityFactory(1, map[string]int{"Kilogram": -1, "Meter": -2, "Second": 4, "Ampere": 2}),
	"farad":     quantityFactory(1, map[string]int{"Kilogram": -1, "Meter": -2, "Second": 4, "Ampere": 2}),
	"F":         quantityFactory(1, map[string]int{"Kilogram": -1, "Meter": -2, "Second": 4, "Ampere": 2}),
	"Ohm":       quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -3, "Ampere": -2}),
	"ohm":       quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -3, "Ampere": -2}),
	"Siemens":   quantityFactory(1, map[string]int{"Kilogram": -1, "Meter": -2, "Second": 3, "Ampere": 2}),
	"siemens":   quantityFactory(1, map[string]int{"Kilogram": -1, "Meter": -2, "Second": 3, "Ampere": 2}),
	"S":         quantityFactory(1, map[string]int{"Kilogram": -1, "Meter": -2, "Second": 3, "Ampere": 2}),
	"Weber":     quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Ampere": -1}),
	"weber":     quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Ampere": -1}),
	"Wb":        quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Ampere": -1}),
	"Tesla":     quantityFactory(1, map[string]int{"Kilogram": 1, "Second": -2, "Ampere": -1}),
	"tesla":     quantityFactory(1, map[string]int{"Kilogram": 1, "Second": -2, "Ampere": -1}),
	"T":         quantityFactory(1, map[string]int{"Kilogram": 1, "Second": -2, "Ampere": -1}),
	"Henry":     quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Ampere": -2}),
	"henry":     quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Ampere": -2}),
	"H":         quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Ampere": -2}),
	"Lumen":     quantityFactory(1, map[string]int{"Candela": 1}),
	"lumen":     quantityFactory(1, map[string]int{"Candela": 1}),
	"lm":        quantityFactory(1, map[string]int{"Candela": 1}),
	"Lux":       quantityFactory(1, map[string]int{"Candela": 1, "Meter": -2}),
	"lux":       quantityFactory(1, map[string]int{"Candela": 1, "Meter": -2}),
	"lx":        quantityFactory(1, map[string]int{"Candela": 1, "Meter": -2}),
	"Becquerel": quantityFactory(1, map[string]int{"Second": -1}),
	"becquerel": quantityFactory(1, map[string]int{"Second": -1}),
	"Bq":        quantityFactory(1, map[string]int{"Second": -1}),
	"Gray":      quantityFactory(1, map[string]int{"Meter": 2, "Second": -2}),
	"gray":      quantityFactory(1, map[string]int{"Meter": 2, "Second": -2}),
	"Gy":        quantityFactory(1, map[string]int{"Meter": 2, "Second": -2}),
	"Sievert":   quantityFactory(1, map[string]int{"Meter": 2, "Second": -2}),
	"sievert":   quantityFactory(1, map[string]int{"Meter": 2, "Second": -2}),
	"Sv":        quantityFactory(1, map[string]int{"Meter": 2, "Second": -2}),
	"Katal":     quantityFactory(1, map[string]int{"Mole": 1, "Second": -1}),
	"katal":     quantityFactory(1, map[string]int{"Mole": 1, "Second": -1}),
	"kat":       quantityFactory(1, map[string]int{"Mole": 1, "Second": -1}),
	"Liter":     quantityFactoryFrac(1, 1000, map[string]int{"Meter": 3}),
	"liter":     quantityFactoryFrac(1, 1000, map[string]int{"Meter": 3}),
	"litre":     quantityFactoryFrac(1, 1000, map[string]int{"Meter": 3}),
	"L":         quantityFactoryFrac(1, 1000, map[string]int{"Meter": 3}),
	"Minute":    quantityFactory(60, map[string]int{"Second": 1}),
	"minute":    quantityFactory(60, map[string]int{"Second": 1}),
	"min":       quantityFactory(60, map[string]int{"Second": 1}),
	"Hour":      quantityFactory(3600, map[string]int{"Second": 1}),
	"hour":      quantityFactory(3600, map[string]int{"Second": 1}),
	"h":         quantityFactory(3600, map[string]int{"Second": 1}),
	"Day":       quantityFactory(86400, map[string]int{"Second": 1}),
	"day":       quantityFactory(86400, map[string]int{"Second": 1}),
	"d":         quantityFactory(86400, map[string]int{"Second": 1}),
	"Week":      quantityFactory(604800, map[string]int{"Second": 1}),
	"week":      quantityFactory(604800, map[string]int{"Second": 1}),

	"Pi":      numberFactory(math.Pi),
	"pi":      numberFactory(math.Pi),
	"Tau":     numberFactory(2 * math.Pi),
	"tau":     numberFactory(2 * math.Pi),
	"E":       numberFactory(math.E),
	"e":       numberFactory(math.E),
	"Phi":     numberFactory((1 + math.Sqrt(5)) / 2),
	"phi":     numberFactory((1 + math.Sqrt(5)) / 2),
	"Percent": numberFactory(0.01),
	"percent": numberFactory(0.01),

	"SpeedOfLight":          numberFactory(299792458),
	"speedOfLight":          numberFactory(299792458),
	"c0":                    numberFactory(299792458),
	"Planck":                quantityFactoryText("6.62607015e-34", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -1}),
	"planck":                quantityFactoryText("6.62607015e-34", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -1}),
	"ReducedPlanck":         quantityFactoryText("1.054571817e-34", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -1}),
	"reducedPlanck":         quantityFactoryText("1.054571817e-34", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -1}),
	"hbar":                  quantityFactoryText("1.054571817e-34", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -1}),
	"Avogadro":              numberFactory(6.02214076e23),
	"avogadro":              numberFactory(6.02214076e23),
	"NA":                    numberFactory(6.02214076e23),
	"Boltzmann":             quantityFactoryText("1.380649e-23", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Kelvin": -1}),
	"boltzmann":             quantityFactoryText("1.380649e-23", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Kelvin": -1}),
	"kB":                    quantityFactoryText("1.380649e-23", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Kelvin": -1}),
	"GasConstant":           quantityFactoryText("8.31446261815324", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Mole": -1, "Kelvin": -1}),
	"gasConstant":           quantityFactoryText("8.31446261815324", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Mole": -1, "Kelvin": -1}),
	"R":                     quantityFactoryText("8.31446261815324", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Mole": -1, "Kelvin": -1}),
	"ElementaryCharge":      quantityFactoryText("1.602176634e-19", map[string]int{"Ampere": 1, "Second": 1}),
	"elementaryCharge":      quantityFactoryText("1.602176634e-19", map[string]int{"Ampere": 1, "Second": 1}),
	"q_e":                   quantityFactoryText("1.602176634e-19", map[string]int{"Ampere": 1, "Second": 1}),
	"GravitationalConstant": quantityFactoryText("6.67430e-11", map[string]int{"Meter": 3, "Kilogram": -1, "Second": -2}),
	"gravitationalConstant": quantityFactoryText("6.67430e-11", map[string]int{"Meter": 3, "Kilogram": -1, "Second": -2}),
	"G":                     quantityFactoryText("6.67430e-11", map[string]int{"Meter": 3, "Kilogram": -1, "Second": -2}),
	"StandardGravity":       quantityFactoryText("9.80665", map[string]int{"Meter": 1, "Second": -2}),
	"standardGravity":       quantityFactoryText("9.80665", map[string]int{"Meter": 1, "Second": -2}),
	"g0":                    quantityFactoryText("9.80665", map[string]int{"Meter": 1, "Second": -2}),
	"FineStructure":         numberFactory(7.2973525693e-3),
	"fineStructure":         numberFactory(7.2973525693e-3),
	"alpha":                 numberFactory(7.2973525693e-3),

	"kilo":  numberFactory(1000),
	"mega":  numberFactory(1000000),
	"giga":  numberFactory(1000000000),
	"tera":  numberFactory(1000000000000),
	"peta":  numberFactory(1000000000000000),
	"exa":   numberFactory(1000000000000000000),
	"milli": numberFactory(0.001),
	"micro": numberFactory(0.000001),
	"nano":  numberFactory(0.000000001),
	"pico":  numberFactory(0.000000000001),
	"centi": numberFactory(0.01),
	"deci":  numberFactory(0.1),
	"deca":  numberFactory(10),
	"hecto": numberFactory(100),
}

// namedConstantValue performs the named constant value step in the surrounding package and returns Value, bool.. It returns Value, bool so callers can continue with the next stage of the pipeline without re-deriving the same information.
func namedConstantValue(name string) (Value, bool) {
	if factory, ok := namedConstantFactories[name]; ok {
		return factory(), true
	}
	return nullValue(), false
}

// namedConstantInfo performs the named constant info step in the surrounding package and returns inferTypeInfo, bool.. It returns inferTypeInfo, bool so callers can continue with the next stage of the pipeline without re-deriving the same information.
func namedConstantInfo(name string) (inferTypeInfo, bool) {
	v, ok := namedConstantValue(name)
	if !ok {
		return inferTypeInfo{}, false
	}
	return inferTypeFromValue(v), true
}

// registerConstants performs the register constants step in the surrounding package and returns no result.. It does not return a value; it communicates success or failure through its side effects or through a separate error path when one exists.
func registerConstants(dst map[string]Value) {
	for name, factory := range namedConstantFactories {
		dst[name] = factory()
	}
}

// unitFactory performs the unit factory step in the surrounding package and returns namedValueFactory.. It returns namedValueFactory so callers can continue with the next stage of the pipeline without re-deriving the same information.
func unitFactory(name string) namedValueFactory {
	return func() Value { return unitValue(name) }
}

// quantityFactory performs the quantity factory step in the surrounding package and returns namedValueFactory.. It returns namedValueFactory so callers can continue with the next stage of the pipeline without re-deriving the same information.
func quantityFactory(mag int64, dims map[string]int) namedValueFactory {
	return func() Value {
		return quantityValueOf(new(big.Rat).SetInt64(mag), dims)
	}
}

// quantityFactoryFrac performs the quantity factory frac step in the surrounding package and returns namedValueFactory.. It returns namedValueFactory so callers can continue with the next stage of the pipeline without re-deriving the same information.
func quantityFactoryFrac(num, den int64, dims map[string]int) namedValueFactory {
	return func() Value {
		return quantityValueOf(new(big.Rat).SetFrac(big.NewInt(num), big.NewInt(den)), dims)
	}
}

// quantityFactoryText performs the quantity factory text step in the surrounding package and returns namedValueFactory.. It returns namedValueFactory so callers can continue with the next stage of the pipeline without re-deriving the same information.
func quantityFactoryText(text string, dims map[string]int) namedValueFactory {
	return func() Value {
		rat, err := parseDecimalText(text)
		if err != nil {
			return nullValue()
		}
		return quantityValueOf(rat, dims)
	}
}

// numberFactory performs the number factory step in the surrounding package and returns namedValueFactory.. It returns namedValueFactory so callers can continue with the next stage of the pipeline without re-deriving the same information.
func numberFactory(v float64) namedValueFactory {
	return func() Value { return numberValue(v) }
}
