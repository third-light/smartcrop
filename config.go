package smartcrop

type Config struct {
	DetailWeight float64

	SkinBias          float64
	SkinBrightnessMin float64
	SkinBrightnessMax float64
	SkinThreshold     float64
	SkinWeight        float64

	SaturationBrightnessMin float64
	SaturationBrightnessMax float64
	SaturationThreshold     float64
	SaturationBias          float64
	SaturationWeight        float64

	ScoreDownSample   int
	Step              int
	ScaleStep         float64
	MinScale          float64
	MaxScale          float64
	EdgeRadius        float64
	EdgeWeight        float64
	OutsideImportance float64
	RuleOfThirds      bool

	Prescale    bool
	PrescaleMin float64

	FaceDetectEnabled        bool
	FaceDetectClassifierFile string
}

var DefaultConfig = Config{
	DetailWeight:             0.2,
	SkinBias:                 0.01,
	SkinBrightnessMin:        0.2,
	SkinBrightnessMax:        1.0,
	SkinThreshold:            0.8,
	SkinWeight:               1.8,
	SaturationBrightnessMin:  0.05,
	SaturationBrightnessMax:  0.9,
	SaturationThreshold:      0.4,
	SaturationBias:           0.2,
	SaturationWeight:         0.3,
	ScoreDownSample:          8, // step * minscale rounded down to the next power of two should be good
	Step:                     8,
	ScaleStep:                0.1,
	MinScale:                 0.9,
	MaxScale:                 1.0,
	EdgeRadius:               0.4,
	EdgeWeight:               -20.0,
	OutsideImportance:        -0.5,
	RuleOfThirds:             true,
	Prescale:                 true,
	PrescaleMin:              400.00,
	FaceDetectEnabled:        false,
	FaceDetectClassifierFile: "",
}

// FaceDetectConfig is a tweaked version of the DefaultConfig that has been optimised for
// smart cropping with face detection enabled.
var FaceDetectConfig = Config{
	DetailWeight:             5.2,
	SkinBias:                 0.01,
	SkinBrightnessMin:        0.2,
	SkinBrightnessMax:        1.0,
	SkinThreshold:            0.8,
	SkinWeight:               5.8,
	SaturationBrightnessMin:  0.05,
	SaturationBrightnessMax:  0.9,
	SaturationThreshold:      0.4,
	SaturationBias:           0.2,
	SaturationWeight:         5.5,
	ScoreDownSample:          2,
	Step:                     8,
	ScaleStep:                0.1,
	MinScale:                 1.0,
	MaxScale:                 1.0,
	EdgeRadius:               0.4,
	EdgeWeight:               -20.0,
	OutsideImportance:        -0.5,
	RuleOfThirds:             true,
	Prescale:                 false,
	PrescaleMin:              400.0,
	FaceDetectEnabled:        true,
	FaceDetectClassifierFile: "", // must be filled in by client
}
