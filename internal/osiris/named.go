package osiris

import (
	"math"
	"math/big"
)

type namedValueFactory func() Value

type namedConstantDefinition struct {
	Canonical string
	Aliases   []string
	Category  string
	Factory   namedValueFactory
}

func namedDefinition(canonical string, aliases []string, category string, factory namedValueFactory) namedConstantDefinition {
	return namedConstantDefinition{Canonical: canonical, Aliases: aliases, Category: category, Factory: factory}
}

var namedConstantDefinitions = []namedConstantDefinition{
	// SI base and coherent derived units follow the BIPM SI Brochure,
	// 9th edition, version 4.01 (June 2026).
	namedDefinition("Meter", []string{"meter", "m"}, "SI Base Units", unitFactory("Meter")),
	namedDefinition("Second", []string{"second", "s"}, "SI Base Units", unitFactory("Second")),
	namedDefinition("Kilogram", []string{"kilogram", "kg"}, "SI Base Units", unitFactory("Kilogram")),
	namedDefinition("Ampere", []string{"ampere", "A"}, "SI Base Units", unitFactory("Ampere")),
	namedDefinition("Kelvin", []string{"kelvin", "K"}, "SI Base Units", unitFactory("Kelvin")),
	namedDefinition("Mole", []string{"mole", "mol"}, "SI Base Units", unitFactory("Mole")),
	namedDefinition("Candela", []string{"candela", "cd"}, "SI Base Units", unitFactory("Candela")),

	namedDefinition("Hertz", []string{"hertz", "Hz"}, "SI Derived Units", quantityFactory(1, map[string]int{"Second": -1})),
	namedDefinition("Newton", []string{"newton", "N"}, "SI Derived Units", quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 1, "Second": -2})),
	namedDefinition("Pascal", []string{"pascal", "Pa"}, "SI Derived Units", quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": -1, "Second": -2})),
	namedDefinition("Joule", []string{"joule", "J"}, "SI Derived Units", quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2})),
	namedDefinition("Watt", []string{"watt", "W"}, "SI Derived Units", quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -3})),
	namedDefinition("Coulomb", []string{"coulomb", "C"}, "SI Derived Units", quantityFactory(1, map[string]int{"Ampere": 1, "Second": 1})),
	namedDefinition("Volt", []string{"volt", "V"}, "SI Derived Units", quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -3, "Ampere": -1})),
	namedDefinition("Farad", []string{"farad", "F"}, "SI Derived Units", quantityFactory(1, map[string]int{"Kilogram": -1, "Meter": -2, "Second": 4, "Ampere": 2})),
	namedDefinition("Ohm", []string{"ohm"}, "SI Derived Units", quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -3, "Ampere": -2})),
	namedDefinition("Siemens", []string{"siemens", "S"}, "SI Derived Units", quantityFactory(1, map[string]int{"Kilogram": -1, "Meter": -2, "Second": 3, "Ampere": 2})),
	namedDefinition("Weber", []string{"weber", "Wb"}, "SI Derived Units", quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Ampere": -1})),
	namedDefinition("Tesla", []string{"tesla", "T"}, "SI Derived Units", quantityFactory(1, map[string]int{"Kilogram": 1, "Second": -2, "Ampere": -1})),
	namedDefinition("Henry", []string{"henry", "H"}, "SI Derived Units", quantityFactory(1, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Ampere": -2})),
	namedDefinition("Lumen", []string{"lumen", "lm"}, "SI Derived Units", quantityFactory(1, map[string]int{"Candela": 1})),
	namedDefinition("Lux", []string{"lux", "lx"}, "SI Derived Units", quantityFactory(1, map[string]int{"Candela": 1, "Meter": -2})),
	namedDefinition("Becquerel", []string{"becquerel", "Bq"}, "SI Derived Units", quantityFactory(1, map[string]int{"Second": -1})),
	namedDefinition("Gray", []string{"gray", "Gy"}, "SI Derived Units", quantityFactory(1, map[string]int{"Meter": 2, "Second": -2})),
	namedDefinition("Sievert", []string{"sievert", "Sv"}, "SI Derived Units", quantityFactory(1, map[string]int{"Meter": 2, "Second": -2})),
	namedDefinition("Katal", []string{"katal", "kat"}, "SI Derived Units", quantityFactory(1, map[string]int{"Mole": 1, "Second": -1})),

	namedDefinition("Liter", []string{"liter", "litre", "L"}, "Metric Units", quantityFactoryFrac(1, 1000, map[string]int{"Meter": 3})),
	namedDefinition("Milliliter", []string{"milliliter", "mL"}, "Metric Units", quantityFactoryText("1e-6", map[string]int{"Meter": 3})),
	namedDefinition("CubicCentimeter", []string{"cubicCentimeter", "cc"}, "Metric Units", quantityFactoryText("1e-6", map[string]int{"Meter": 3})),
	namedDefinition("Gram", []string{"gram", "g"}, "Metric Units", quantityFactoryText("1e-3", map[string]int{"Kilogram": 1})),
	namedDefinition("MetricTonne", []string{"metricTonne", "tonne"}, "Metric Units", quantityFactory(1000, map[string]int{"Kilogram": 1})),
	namedDefinition("Hectare", []string{"hectare", "ha"}, "Metric Units", quantityFactory(10000, map[string]int{"Meter": 2})),
	namedDefinition("Angstrom", []string{"angstrom"}, "Metric Units", quantityFactoryText("1e-10", map[string]int{"Meter": 1})),

	// International length and customary-unit definitions are exact. The
	// inch/pound family follows NIST SP 811 (2008), Web version 3.2; U.S.
	// liquid measures use the exact 231 cubic-inch gallon. Imperial volume
	// uses the exact 4.54609 L gallon from the UK Weights and Measures Act.
	namedDefinition("Inch", []string{"inch"}, "Customary Length", quantityFactoryText("0.0254", map[string]int{"Meter": 1})),
	namedDefinition("Foot", []string{"foot", "ft"}, "Customary Length", quantityFactoryText("0.3048", map[string]int{"Meter": 1})),
	namedDefinition("Yard", []string{"yard", "yd"}, "Customary Length", quantityFactoryText("0.9144", map[string]int{"Meter": 1})),
	namedDefinition("Mile", []string{"mile", "mi"}, "Customary Length", quantityFactoryText("1609.344", map[string]int{"Meter": 1})),
	namedDefinition("NauticalMile", []string{"nauticalMile", "nmi"}, "Customary Length", quantityFactory(1852, map[string]int{"Meter": 1})),
	namedDefinition("Acre", []string{"acre"}, "Area and Volume", quantityFactoryText("4046.8564224", map[string]int{"Meter": 2})),
	namedDefinition("Gallon", []string{"gallon", "gal"}, "Area and Volume", quantityFactoryText("0.003785411784", map[string]int{"Meter": 3})),
	namedDefinition("USQuart", []string{"usQuart", "qt"}, "Area and Volume", quantityFactoryText("0.000946352946", map[string]int{"Meter": 3})),
	namedDefinition("USPint", []string{"usPint", "pt"}, "Area and Volume", quantityFactoryText("0.000473176473", map[string]int{"Meter": 3})),
	namedDefinition("USCup", []string{"usCup", "cup"}, "Area and Volume", quantityFactoryText("0.0002365882365", map[string]int{"Meter": 3})),
	namedDefinition("USFluidOunce", []string{"usFluidOunce", "fl_oz"}, "Area and Volume", quantityFactoryText("0.0000295735295625", map[string]int{"Meter": 3})),
	namedDefinition("USTablespoon", []string{"usTablespoon", "tbsp"}, "Area and Volume", quantityFactoryText("0.00001478676478125", map[string]int{"Meter": 3})),
	namedDefinition("USTeaspoon", []string{"usTeaspoon", "tsp"}, "Area and Volume", quantityFactoryText("0.00000492892159375", map[string]int{"Meter": 3})),
	namedDefinition("ImperialGallon", []string{"imperialGallon", "imp_gal"}, "Area and Volume", quantityFactoryText("0.00454609", map[string]int{"Meter": 3})),
	namedDefinition("ImperialQuart", []string{"imperialQuart"}, "Area and Volume", quantityFactoryText("0.0011365225", map[string]int{"Meter": 3})),
	namedDefinition("ImperialPint", []string{"imperialPint"}, "Area and Volume", quantityFactoryText("0.00056826125", map[string]int{"Meter": 3})),
	namedDefinition("ImperialFluidOunce", []string{"imperialFluidOunce"}, "Area and Volume", quantityFactoryText("0.0000284130625", map[string]int{"Meter": 3})),
	namedDefinition("Ounce", []string{"ounce", "oz"}, "Customary Mass", quantityFactoryText("0.028349523125", map[string]int{"Kilogram": 1})),
	namedDefinition("Pound", []string{"pound", "lb"}, "Customary Mass", quantityFactoryText("0.45359237", map[string]int{"Kilogram": 1})),
	namedDefinition("Stone", []string{"stone", "st"}, "Customary Mass", quantityFactoryText("6.35029318", map[string]int{"Kilogram": 1})),
	namedDefinition("ShortTon", []string{"shortTon"}, "Customary Mass", quantityFactoryText("907.18474", map[string]int{"Kilogram": 1})),
	namedDefinition("LongTon", []string{"longTon"}, "Customary Mass", quantityFactoryText("1016.0469088", map[string]int{"Kilogram": 1})),

	// The astronomical unit and Julian year are exact BIPM/IAU definitions.
	// LightYear is their exact product with c. Parsec is the IAU mathematical
	// definition 648000/pi au, represented by the published decimal below.
	namedDefinition("AstronomicalUnit", []string{"astronomicalUnit", "au"}, "Astronomical Units", quantityFactory(149597870700, map[string]int{"Meter": 1})),
	namedDefinition("LightYear", []string{"lightYear", "ly"}, "Astronomical Units", quantityFactoryText("9460730472580800", map[string]int{"Meter": 1})),
	namedDefinition("Parsec", []string{"parsec", "pc"}, "Astronomical Units", quantityFactoryText("3.0856775814913673e16", map[string]int{"Meter": 1})),
	namedDefinition("Dalton", []string{"dalton", "Da"}, "Astronomical Units", quantityFactoryText("1.66053906892e-27", map[string]int{"Kilogram": 1})),

	namedDefinition("Minute", []string{"minute", "min"}, "Time Units", quantityFactory(60, map[string]int{"Second": 1})),
	namedDefinition("Hour", []string{"hour", "h"}, "Time Units", quantityFactory(3600, map[string]int{"Second": 1})),
	namedDefinition("Day", []string{"day", "d"}, "Time Units", quantityFactory(86400, map[string]int{"Second": 1})),
	namedDefinition("Week", []string{"week"}, "Time Units", quantityFactory(604800, map[string]int{"Second": 1})),
	namedDefinition("Nanosecond", []string{"nanosecond", "ns"}, "Time Units", quantityFactoryText("1e-9", map[string]int{"Second": 1})),
	namedDefinition("Microsecond", []string{"microsecond", "us"}, "Time Units", quantityFactoryText("1e-6", map[string]int{"Second": 1})),
	namedDefinition("Millisecond", []string{"millisecond", "ms"}, "Time Units", quantityFactoryText("1e-3", map[string]int{"Second": 1})),
	namedDefinition("JulianYear", []string{"julianYear"}, "Time Units", quantityFactory(31557600, map[string]int{"Second": 1})),

	namedDefinition("Knot", []string{"knot", "kn"}, "Speed and Frequency", quantityFactoryText("463/900", map[string]int{"Meter": 1, "Second": -1})),
	namedDefinition("KilometerPerHour", []string{"kilometerPerHour", "kph"}, "Speed and Frequency", quantityFactoryText("5/18", map[string]int{"Meter": 1, "Second": -1})),
	namedDefinition("MilePerHour", []string{"milePerHour", "mph"}, "Speed and Frequency", quantityFactoryText("1397/3125", map[string]int{"Meter": 1, "Second": -1})),
	namedDefinition("RevolutionPerMinute", []string{"revolutionPerMinute", "rpm"}, "Speed and Frequency", quantityFactoryText("1/60", map[string]int{"Second": -1})),

	namedDefinition("Bar", []string{"bar"}, "Pressure", quantityFactory(100000, map[string]int{"Kilogram": 1, "Meter": -1, "Second": -2})),
	namedDefinition("Millibar", []string{"millibar", "mbar"}, "Pressure", quantityFactory(100, map[string]int{"Kilogram": 1, "Meter": -1, "Second": -2})),
	namedDefinition("Atmosphere", []string{"atmosphere", "atm"}, "Pressure", quantityFactory(101325, map[string]int{"Kilogram": 1, "Meter": -1, "Second": -2})),
	namedDefinition("Torr", []string{"torr"}, "Pressure", quantityFactoryText("20265/152", map[string]int{"Kilogram": 1, "Meter": -1, "Second": -2})),
	namedDefinition("PoundsPerSquareInch", []string{"poundsPerSquareInch", "psi"}, "Pressure", quantityFactoryText("8896443230521/1290320000", map[string]int{"Kilogram": 1, "Meter": -1, "Second": -2})),

	namedDefinition("Electronvolt", []string{"electronvolt", "eV"}, "Energy and Power", quantityFactoryText("1.602176634e-19", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2})),
	namedDefinition("Calorie", []string{"calorie", "cal"}, "Energy and Power", quantityFactoryText("4.184", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2})),
	namedDefinition("Kilocalorie", []string{"kilocalorie", "kcal"}, "Energy and Power", quantityFactory(4184, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2})),
	namedDefinition("BritishThermalUnit", []string{"britishThermalUnit", "BTU"}, "Energy and Power", quantityFactoryText("1055.05585262", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2})),
	namedDefinition("KilowattHour", []string{"kilowattHour", "kWh"}, "Energy and Power", quantityFactory(3600000, map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2})),
	namedDefinition("MechanicalHorsepower", []string{"mechanicalHorsepower", "hp"}, "Energy and Power", quantityFactoryText("37284993579113511/50000000000000", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -3})),

	// Plane angle remains dimensionless in the quantity model. These are
	// default-number scales, rounded once to binary64.
	namedDefinition("Radian", []string{"radian", "rad"}, "Angle Scales", numberFactory(1)),
	namedDefinition("Degree", []string{"degree", "deg"}, "Angle Scales", numberFactory(math.Pi/180)),
	namedDefinition("Arcminute", []string{"arcminute"}, "Angle Scales", numberFactory(math.Pi/10800)),
	namedDefinition("Arcsecond", []string{"arcsecond"}, "Angle Scales", numberFactory(math.Pi/648000)),
	namedDefinition("Revolution", []string{"revolution", "turn"}, "Angle Scales", numberFactory(2*math.Pi)),

	namedDefinition("Pi", []string{"pi"}, "Mathematical Constants", numberFactory(math.Pi)),
	namedDefinition("Tau", []string{"tau"}, "Mathematical Constants", numberFactory(2*math.Pi)),
	namedDefinition("E", []string{"e"}, "Mathematical Constants", numberFactory(math.E)),
	namedDefinition("Phi", []string{"phi"}, "Mathematical Constants", numberFactory((1+math.Sqrt(5))/2)),
	namedDefinition("Sqrt2", []string{"sqrt2"}, "Mathematical Constants", numberFactory(math.Sqrt2)),
	namedDefinition("SqrtHalf", []string{"sqrtHalf"}, "Mathematical Constants", numberFactory(math.Sqrt2/2)),
	namedDefinition("Ln2", []string{"ln2"}, "Mathematical Constants", numberFactory(math.Ln2)),
	namedDefinition("Ln10", []string{"ln10"}, "Mathematical Constants", numberFactory(math.Ln10)),
	namedDefinition("Log2E", []string{"log2E"}, "Mathematical Constants", numberFactory(math.Log2E)),
	namedDefinition("Log10E", []string{"log10E"}, "Mathematical Constants", numberFactory(math.Log10E)),
	namedDefinition("EulerGamma", []string{"eulerGamma"}, "Mathematical Constants", numberFactory(0.5772156649015329)),
	namedDefinition("Catalan", []string{"catalan"}, "Mathematical Constants", numberFactory(0.915965594177219)),
	namedDefinition("Apery", []string{"apery"}, "Mathematical Constants", numberFactory(1.2020569031595942)),
	namedDefinition("Percent", []string{"percent"}, "Mathematical Constants", numberFactory(0.01)),

	// Physical constants use NIST's 2022 CODATA recommended values, database
	// version 9.0 (May 2024). Exact defined constants and measured concise
	// values are intentionally distinct in the documentation.
	namedDefinition("SpeedOfLight", []string{"speedOfLight", "c0"}, "Physical Constants", quantityFactory(299792458, map[string]int{"Meter": 1, "Second": -1})),
	namedDefinition("Planck", []string{"planck"}, "Physical Constants", quantityFactoryText("6.62607015e-34", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -1})),
	namedDefinition("ReducedPlanck", []string{"reducedPlanck", "hbar"}, "Physical Constants", quantityFactoryText("1.054571817e-34", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -1})),
	namedDefinition("Avogadro", []string{"avogadro", "NA"}, "Physical Constants", quantityFactoryText("6.02214076e23", map[string]int{"Mole": -1})),
	namedDefinition("Boltzmann", []string{"boltzmann", "kB"}, "Physical Constants", quantityFactoryText("1.380649e-23", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Kelvin": -1})),
	namedDefinition("GasConstant", []string{"gasConstant", "R"}, "Physical Constants", quantityFactoryText("8.31446261815324", map[string]int{"Kilogram": 1, "Meter": 2, "Second": -2, "Mole": -1, "Kelvin": -1})),
	namedDefinition("ElementaryCharge", []string{"elementaryCharge", "q_e"}, "Physical Constants", quantityFactoryText("1.602176634e-19", map[string]int{"Ampere": 1, "Second": 1})),
	namedDefinition("GravitationalConstant", []string{"gravitationalConstant", "G"}, "Physical Constants", quantityFactoryText("6.67430e-11", map[string]int{"Meter": 3, "Kilogram": -1, "Second": -2})),
	namedDefinition("StandardGravity", []string{"standardGravity", "g0"}, "Physical Constants", quantityFactoryText("9.80665", map[string]int{"Meter": 1, "Second": -2})),
	namedDefinition("FineStructure", []string{"fineStructure", "alpha"}, "Physical Constants", numberFactory(7.2973525643e-3)),
	namedDefinition("VacuumPermeability", []string{"vacuumPermeability", "mu0"}, "Physical Constants", quantityFactoryText("1.25663706127e-6", map[string]int{"Kilogram": 1, "Meter": 1, "Second": -2, "Ampere": -2})),
	namedDefinition("VacuumPermittivity", []string{"vacuumPermittivity", "epsilon0"}, "Physical Constants", quantityFactoryText("8.8541878188e-12", map[string]int{"Kilogram": -1, "Meter": -3, "Second": 4, "Ampere": 2})),
	namedDefinition("ElectronMass", []string{"electronMass", "m_e"}, "Physical Constants", quantityFactoryText("9.1093837139e-31", map[string]int{"Kilogram": 1})),
	namedDefinition("ProtonMass", []string{"protonMass", "m_p"}, "Physical Constants", quantityFactoryText("1.67262192595e-27", map[string]int{"Kilogram": 1})),
	namedDefinition("NeutronMass", []string{"neutronMass", "m_n"}, "Physical Constants", quantityFactoryText("1.67492750056e-27", map[string]int{"Kilogram": 1})),
	namedDefinition("AtomicMassConstant", []string{"atomicMassConstant", "m_u"}, "Physical Constants", quantityFactoryText("1.66053906892e-27", map[string]int{"Kilogram": 1})),
	namedDefinition("FaradayConstant", []string{"faradayConstant"}, "Physical Constants", quantityFactoryText("96485.3321233100184", map[string]int{"Ampere": 1, "Second": 1, "Mole": -1})),
	namedDefinition("StefanBoltzmannConstant", []string{"stefanBoltzmannConstant", "sigmaSB"}, "Physical Constants", quantityFactoryText("5.670374419e-8", map[string]int{"Kilogram": 1, "Second": -3, "Kelvin": -4})),
	namedDefinition("RydbergConstant", []string{"rydbergConstant", "Rinf"}, "Physical Constants", quantityFactoryText("10973731.568157", map[string]int{"Meter": -1})),
	namedDefinition("BohrRadius", []string{"bohrRadius", "a0"}, "Physical Constants", quantityFactoryText("5.29177210544e-11", map[string]int{"Meter": 1})),
	namedDefinition("BohrMagneton", []string{"bohrMagneton", "muB"}, "Physical Constants", quantityFactoryText("9.2740100657e-24", map[string]int{"Ampere": 1, "Meter": 2})),
	namedDefinition("NuclearMagneton", []string{"nuclearMagneton", "muN"}, "Physical Constants", quantityFactoryText("5.0507837393e-27", map[string]int{"Ampere": 1, "Meter": 2})),

	namedDefinition("pico", nil, "Metric Prefixes", numberFactory(0.000000000001)),
	namedDefinition("nano", nil, "Metric Prefixes", numberFactory(0.000000001)),
	namedDefinition("micro", nil, "Metric Prefixes", numberFactory(0.000001)),
	namedDefinition("milli", nil, "Metric Prefixes", numberFactory(0.001)),
	namedDefinition("centi", nil, "Metric Prefixes", numberFactory(0.01)),
	namedDefinition("deci", nil, "Metric Prefixes", numberFactory(0.1)),
	namedDefinition("deca", nil, "Metric Prefixes", numberFactory(10)),
	namedDefinition("hecto", nil, "Metric Prefixes", numberFactory(100)),
	namedDefinition("kilo", nil, "Metric Prefixes", numberFactory(1000)),
	namedDefinition("mega", nil, "Metric Prefixes", numberFactory(1000000)),
	namedDefinition("giga", nil, "Metric Prefixes", numberFactory(1000000000)),
	namedDefinition("tera", nil, "Metric Prefixes", numberFactory(1000000000000)),
	namedDefinition("peta", nil, "Metric Prefixes", numberFactory(1000000000000000)),
	namedDefinition("exa", nil, "Metric Prefixes", numberFactory(1000000000000000000)),

	namedDefinition("UnknownError", nil, "Error Kinds", errorKindFactory("unknown_error")),
	namedDefinition("InvalidArgument", nil, "Error Kinds", errorKindFactory("invalid_argument")),
	namedDefinition("NotFound", nil, "Error Kinds", errorKindFactory("not_found")),
	namedDefinition("AlreadyExists", nil, "Error Kinds", errorKindFactory("already_exists")),
	namedDefinition("PermissionDenied", nil, "Error Kinds", errorKindFactory("permission_denied")),
	namedDefinition("Unauthenticated", nil, "Error Kinds", errorKindFactory("unauthenticated")),
	namedDefinition("Unavailable", nil, "Error Kinds", errorKindFactory("unavailable")),
	namedDefinition("Timeout", nil, "Error Kinds", errorKindFactory("timeout")),
	namedDefinition("Cancelled", nil, "Error Kinds", errorKindFactory("cancelled")),
	namedDefinition("Conflict", nil, "Error Kinds", errorKindFactory("conflict")),
	namedDefinition("ResourceExhausted", nil, "Error Kinds", errorKindFactory("resource_exhausted")),
	namedDefinition("OutOfMemory", nil, "Error Kinds", errorKindFactory("out_of_memory")),
	namedDefinition("OutOfRange", nil, "Error Kinds", errorKindFactory("out_of_range")),
	namedDefinition("Unsupported", nil, "Error Kinds", errorKindFactory("unsupported")),
	namedDefinition("NotImplemented", nil, "Error Kinds", errorKindFactory("not_implemented")),
	namedDefinition("ParseError", nil, "Error Kinds", errorKindFactory("parse_error")),
	namedDefinition("TypeError", nil, "Error Kinds", errorKindFactory("type_error")),
	namedDefinition("EncodingError", nil, "Error Kinds", errorKindFactory("encoding_error")),
	namedDefinition("IOError", nil, "Error Kinds", errorKindFactory("io_error")),
	namedDefinition("NetworkError", nil, "Error Kinds", errorKindFactory("network_error")),
	namedDefinition("DatabaseError", nil, "Error Kinds", errorKindFactory("database_error")),
	namedDefinition("InternalError", nil, "Error Kinds", errorKindFactory("internal_error")),
}

var namedConstantFactories = buildNamedConstantFactories(namedConstantDefinitions)

func buildNamedConstantFactories(definitions []namedConstantDefinition) map[string]namedValueFactory {
	factories := make(map[string]namedValueFactory)
	for _, definition := range definitions {
		for _, name := range append([]string{definition.Canonical}, definition.Aliases...) {
			if name == "" || definition.Factory == nil {
				panic("invalid named constant definition")
			}
			if _, exists := factories[name]; exists {
				panic("duplicate named constant " + name)
			}
			factories[name] = definition.Factory
		}
	}
	return factories
}

// namedConstantValue handles the named constant value step in the surrounding
// package and returns Value, bool.
//
// It returns Value, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func namedConstantValue(name string) (Value, bool) {
	if factory, ok := namedConstantFactories[name]; ok {
		return factory(), true
	}
	return nullValue(), false
}

// namedConstantInfo handles the named constant info step in the surrounding
// package and returns inferTypeInfo, bool.
//
// It returns inferTypeInfo, bool so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
func namedConstantInfo(name string) (inferTypeInfo, bool) {
	v, ok := namedConstantValue(name)
	if !ok {
		return inferTypeInfo{}, false
	}
	return inferTypeFromValue(v), true
}

// registerConstants handles the register constants step in the surrounding
// package and returns no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func registerConstants(dst map[string]Value) {
	for _, definition := range namedConstantDefinitions {
		dst[definition.Canonical] = definition.Factory()
		for _, alias := range definition.Aliases {
			dst[alias] = definition.Factory()
		}
	}
}

// unitFactory handles the unit factory step in the surrounding package and
// returns namedValueFactory.
//
// It returns namedValueFactory so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func unitFactory(name string) namedValueFactory {
	return func() Value { return unitValue(name) }
}

// quantityFactory handles the quantity factory step in the surrounding package
// and returns namedValueFactory.
//
// It returns namedValueFactory so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func quantityFactory(mag int64, dims map[string]int) namedValueFactory {
	return func() Value {
		return quantityValueOf(new(big.Rat).SetInt64(mag), dims)
	}
}

// quantityFactoryFrac handles the quantity factory frac step in the
// surrounding package and returns namedValueFactory.
//
// It returns namedValueFactory so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func quantityFactoryFrac(num, den int64, dims map[string]int) namedValueFactory {
	return func() Value {
		return quantityValueOf(new(big.Rat).SetFrac(big.NewInt(num), big.NewInt(den)), dims)
	}
}

// quantityFactoryText handles the quantity factory text step in the
// surrounding package and returns namedValueFactory.
//
// It returns namedValueFactory so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func quantityFactoryText(text string, dims map[string]int) namedValueFactory {
	return func() Value {
		rat, err := parseDecimalText(text)
		if err != nil {
			return nullValue()
		}
		return quantityValueOf(rat, dims)
	}
}

// numberFactory handles the number factory step in the surrounding package and
// returns namedValueFactory.
//
// It returns namedValueFactory so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func numberFactory(v float64) namedValueFactory {
	return func() Value { return numberValue(v) }
}

// errorKindFactory handles the error kind factory step in the surrounding
// package and returns namedValueFactory.
//
// It returns namedValueFactory so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func errorKindFactory(code string) namedValueFactory {
	return func() Value { return stringValue(code) }
}
