package field

type fieldOption func(SchemaField) SchemaField

func WithRequired(required bool) fieldOption {
	return func(o SchemaField) SchemaField {
		o.Required = required
		return o
	}
}

func WithDescription(description string) fieldOption {
	return func(o SchemaField) SchemaField {
		o.Description = description

		return o
	}
}

func WithDisplayName(displayName string) fieldOption {
	return func(o SchemaField) SchemaField {
		o.ConnectorConfig.DisplayName = displayName
		return o
	}
}

func WithDefaultValue(value any) fieldOption {
	return func(o SchemaField) SchemaField {
		o.DefaultValue = value

		return o
	}
}

func WithHidden(hidden bool) fieldOption {
	return func(o SchemaField) SchemaField {
		o.SyncerConfig.Hidden = hidden
		o.ConnectorConfig.Ignore = true
		return o
	}
}

func WithIgnoreSyncer(value bool) fieldOption {
	return func(o SchemaField) SchemaField {
		o.SyncerConfig.Ignore = value
		return o
	}
}

func WithIsOps() fieldOption {
	return func(o SchemaField) SchemaField {
		o.Ops = true
		return o
	}
}

func WithConnector(value bool) fieldOption {
	return func(o SchemaField) SchemaField {
		o.ConnectorConfig.Ignore = !value
		return o
	}
}

func WithRequiredConnector(value bool) fieldOption {
	return func(o SchemaField) SchemaField {
		o.ConnectorConfig.Required = value
		o.ConnectorConfig.Ignore = false
		return o
	}
}

func WithConnectorConf(wc ConnectorConfig) fieldOption {
	return func(o SchemaField) SchemaField {
		o.ConnectorConfig = wc
		return o
	}
}

func WithShortHand(sh string) fieldOption {
	return func(o SchemaField) SchemaField {
		o.SyncerConfig.ShortHand = sh

		return o
	}
}

func WithPersistent(value bool) fieldOption {
	return func(o SchemaField) SchemaField {
		o.SyncerConfig.Persistent = value

		return o
	}
}

func WithIsSecret(value bool) fieldOption {
	return func(o SchemaField) SchemaField {
		o.Secret = value

		return o
	}
}

func WithPlaceholder(value string) fieldOption {
	return func(o SchemaField) SchemaField {
		o.ConnectorConfig.Placeholder = value

		return o
	}
}

func WithWebUI(displayName string, placeholder string) fieldOption {
	return func(o SchemaField) SchemaField {
		o.ConnectorConfig.DisplayName = displayName
		o.ConnectorConfig.Placeholder = placeholder

		return o
	}
}
