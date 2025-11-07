package basketball_nba

// FeaturedMarkets returns the list of featured (mainline) markets for NBA
func FeaturedMarkets() []string {
	return []string{"h2h", "spreads", "totals"}
}

// PropsMarkets returns the list of player prop markets for NBA
func PropsMarkets() []string {
	return []string{
		"player_points",
		"player_rebounds",
		"player_assists",
		"player_threes",
		"player_points_rebounds_assists",
		"player_points_rebounds",
		"player_points_assists",
		"player_rebounds_assists",
		"player_steals",
		"player_blocks",
		"player_turnovers",
		"player_double_double",
		"player_triple_double",
	}
}

// MapVendorMarketKey translates vendor market keys to internal keys
// For The Odds API, these are already 1:1, but this allows for future adapters
func MapVendorMarketKey(vendorKey string) string {
	// The Odds API uses same keys as our internal schema
	return vendorKey
}

// IsPropsMarket returns true if the market is a player prop
func IsPropsMarket(marketKey string) bool {
	propsMap := make(map[string]bool)
	for _, m := range PropsMarkets() {
		propsMap[m] = true
	}
	return propsMap[marketKey]
}

