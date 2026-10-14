package planner

// Monetary values are decimal strings at every API boundary.
type Request struct {
	Mode             string      `json:"mode"`
	Symbol           string      `json:"symbol"`
	Side             string      `json:"side"`
	Entry            string      `json:"entry"`
	Quantity         string      `json:"quantity"`
	Risk             string      `json:"risk"`
	RiskUnit         string      `json:"riskUnit"`
	AccountReference string      `json:"accountReference"`
	AccountBasis     string      `json:"accountBasis"`
	RewardRatio      string      `json:"rewardRatio"`
	SpacingPercent   string      `json:"spacingPercent"`
	Leverage         int         `json:"leverage"`
	MaxLevels        int         `json:"maxLevels"`
	MaxMargin        string      `json:"maxMargin"`
	CostReserve      string      `json:"costReserve"`
	Hedge            *HedgeInput `json:"hedge,omitempty"`
}

type HedgeInput struct {
	LongEntry     string `json:"longEntry"`
	ShortEntry    string `json:"shortEntry"`
	LongQuantity  string `json:"longQuantity"`
	ShortQuantity string `json:"shortQuantity"`
	MarkPrice     string `json:"markPrice"`
	RealizedPnl   string `json:"realizedPnl"`
	FeesPaid      string `json:"feesPaid"`
	FundingNet    string `json:"fundingNet"`
	ClosePercent  string `json:"closePercent"`
}

type Level struct {
	Index              int     `json:"index"`
	Price              string  `json:"price"`
	Quantity           string  `json:"quantity"`
	CumulativeQuantity string  `json:"cumulativeQuantity"`
	AveragePrice       string  `json:"averagePrice"`
	Margin             string  `json:"margin"`
	StopPrice          *string `json:"stopPrice"`
	TakeProfit         *string `json:"takeProfit"`
	TakeProfitQuantity string  `json:"takeProfitQuantity"`
	NetExposure        string  `json:"netExposure"`
}

type Plan struct {
	Mode             string   `json:"mode"`
	Symbol           string   `json:"symbol"`
	Side             string   `json:"side"`
	Simulation       bool     `json:"simulation"`
	Risk             string   `json:"risk"`
	RiskInput        string   `json:"riskInput"`
	RiskUnit         string   `json:"riskUnit"`
	AccountReference string   `json:"accountReference"`
	AccountBasis     string   `json:"accountBasis"`
	CostReserve      string   `json:"costReserve"`
	BaselinePnl      *string  `json:"baselinePnl"`
	PnlThreshold     *string  `json:"pnlThreshold"`
	SpacingPercent   string   `json:"spacingPercent"`
	Levels           []Level  `json:"levels"`
	Warnings         []string `json:"warnings"`
	StopReason       string   `json:"stopReason"`
}
