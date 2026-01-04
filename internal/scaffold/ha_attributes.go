package scaffold

// getStandardHAAttributes returns standard attributes for each HA component type
// Based on Home Assistant MQTT Discovery documentation
func getStandardHAAttributes(componentType string) []string {
	switch componentType {
	case "climate":
		return []string{
			"current_temperature",
			"target_temperature",
			"hvac_mode",
			"hvac_action",
			"fan_mode",
			"preset_mode",
		}
	case "humidifier":
		return []string{
			"current_humidity",
			"target_humidity",
			"mode",
			"action",
		}
	case "fan":
		return []string{
			"state",
			"percentage",
			"preset_mode",
			"oscillating",
		}
	case "cover":
		return []string{
			"state",
			"position",
			"tilt_position",
		}
	case "valve":
		return []string{
			"state",
			"position",
		}
	case "lock":
		return []string{
			"state",
		}
	case "vacuum":
		return []string{
			"state",
			"battery_level",
			"fan_speed",
		}
	case "alarm":
		return []string{
			"state",
		}
	case "water_heater":
		return []string{
			"current_temperature",
			"target_temperature",
			"mode",
		}
	case "lawn_mower":
		return []string{
			"state",
			"battery_level",
		}
	case "light":
		return []string{
			"state",
			"brightness",
			"color_temp",
			"rgb_color",
		}
	case "switch":
		return []string{
			"state",
		}
	case "sensor":
		return []string{
			"value",
		}
	case "binary_sensor":
		return []string{
			"state",
		}
	case "number":
		return []string{
			"value",
		}
	case "select":
		return []string{
			"state",
		}
	case "text":
		return []string{
			"value",
		}
	default:
		return []string{"state"}
	}
}
