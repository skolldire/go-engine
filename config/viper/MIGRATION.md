# Migration

This source is optional. Supply FS and ordered paths explicitly; no CONF_DIR,
cwd discovery or AutomaticEnv. Pass the Source to the config Loader and use
env.Resolver with an explicit lookup. The application decodes over tool defaults,
validates and constructs the tool. Engine's reader remains unchanged.

Native Viper lowercasing, dotted-key splitting and JSON float64 precision remain
accepted limitations. Use config/json for literal keys/exact decimals. Required
true makes missing files errors; false skips only fs.ErrNotExist. No semantic
compatibility alias or automatic engine migration is provided.
