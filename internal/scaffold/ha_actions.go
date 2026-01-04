package scaffold

// getStandardHAActions returns standard actions for each HA component type
// These are created in scaffold even if device has no command_topic
func getStandardHAActions(componentType string) []string {
	switch componentType {
	case "climate":
		return []string{
			"turn_on",
			"turn_off",
			"set_temperature",
			"set_hvac_mode",
			"set_fan_mode",
			"set_preset_mode",
		}
	case "humidifier":
		return []string{
			"turn_on",
			"turn_off",
			"set_humidity",
			"set_mode",
		}
	case "fan":
		return []string{
			"turn_on",
			"turn_off",
			"set_percentage",
			"set_preset_mode",
		}
	case "cover", "valve":
		return []string{
			"open",
			"close",
			"stop",
			"set_position",
		}
	case "lock":
		return []string{
			"lock",
			"unlock",
		}
	case "vacuum":
		return []string{
			"start",
			"pause",
			"stop",
			"return_to_base",
		}
	case "alarm":
		return []string{
			"arm_away",
			"arm_home",
			"disarm",
		}
	case "water_heater":
		return []string{
			"set_temperature",
			"set_mode",
		}
	case "lawn_mower":
		return []string{
			"start_mowing",
			"pause",
			"dock",
		}
	case "light":
		return []string{
			"turn_on",
			"turn_off",
			"toggle",
			"set_brightness",
			"set_color",
		}
	case "switch":
		return []string{
			"turn_on",
			"turn_off",
			"toggle",
		}
	case "siren":
		return []string{
			"turn_on",
			"turn_off",
		}
	case "number":
		return []string{
			"set_value",
		}
	case "select":
		return []string{
			"select_option",
		}
	case "text":
		return []string{
			"set_value",
		}
	default:
		return []string{}
	}
}
