func (ctx *httpContext) OnHttpResponseHeaders(numHeaders int, endOfStream bool) types.Action {
	proxywasm.LogInfo("OnHttpResponseHeaders")

	key := "x-custom-header"
	value := "custom-value"

	if err := proxywasm.AddHttpResponseHeader(key, value); err != nil {
		proxywasm.LogCriticalf("failed to add header: %v", err)
		return types.ActionPause
	}
	proxywasm.LogInfof("header set: %s=%s", key, value)
	return types.ActionContinue
}