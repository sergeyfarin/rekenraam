package api

import (
	"rekenraam/backend/internal/db"
	"rekenraam/backend/internal/exact"
)

// gainImpactResponse is the replay gain disclosure contract (T-114). It names
// committed disposals by durable IDs only; replacement results carry values.
type gainImpactResponse struct {
	Changes         []gainImpactChangeResponse `json:"changes"`
	Acknowledgement string                     `json:"acknowledgement"`
}

type gainImpactChangeResponse struct {
	ChangeKind      string                   `json:"change_kind"`
	RootOperationID int64                    `json:"root_operation_id"`
	DecisionSeq     int                      `json:"decision_seq"`
	OperationID     int64                    `json:"operation_id"`
	DecisionID      int64                    `json:"decision_id"`
	TransactionID   int64                    `json:"transaction_id"`
	Before          gainImpactStateResponse  `json:"before"`
	After           *gainImpactStateResponse `json:"after"`
}

type gainImpactStateResponse struct {
	AccountID          int64              `json:"account_id"`
	CommodityID        int64              `json:"commodity_id"`
	CostCommodityID    int64              `json:"cost_commodity_id"`
	DisposalDate       string             `json:"disposal_date"`
	CostBasisMethod    string             `json:"cost_basis_method"`
	QuantityValue      exact.Coefficient  `json:"quantity_value"`
	QuantityScale      int                `json:"quantity_scale"`
	BasisKnowledge     string             `json:"basis_knowledge"`
	DisposedBasisValue *exact.Coefficient `json:"disposed_basis_value"`
	DisposedBasisScale *int               `json:"disposed_basis_scale"`
	ProceedsValue      exact.Coefficient  `json:"proceeds_value"`
	ProceedsScale      int                `json:"proceeds_scale"`
	RealizedGainValue  *exact.Coefficient `json:"realized_gain_value"`
	RealizedGainScale  *int               `json:"realized_gain_scale"`
}

// toGainImpactResponse returns nil for a command that has not opted in. A
// value wider than the coefficient contract is reported as ledger overflow.
func toGainImpactResponse(impact *db.InvestmentGainImpact) (*gainImpactResponse, error) {
	if impact == nil {
		return nil, nil
	}
	response := &gainImpactResponse{Changes: make([]gainImpactChangeResponse, 0, len(impact.Changes)),
		Acknowledgement: impact.Acknowledgement}
	for _, change := range impact.Changes {
		before, err := toGainImpactStateResponse(change.Before)
		if err != nil {
			return nil, err
		}
		item := gainImpactChangeResponse{ChangeKind: change.Kind,
			RootOperationID: change.Identity.RootOperationID, DecisionSeq: change.Identity.DecisionSeq,
			OperationID: change.OperationID, DecisionID: change.DecisionID,
			TransactionID: change.TransactionID, Before: before}
		if change.After != nil {
			after, err := toGainImpactStateResponse(*change.After)
			if err != nil {
				return nil, err
			}
			item.After = &after
		}
		response.Changes = append(response.Changes, item)
	}
	return response, nil
}

func toGainImpactStateResponse(state db.InvestmentGainState) (gainImpactStateResponse, error) {
	quantity, err := state.Quantity.Coefficient()
	if err != nil {
		return gainImpactStateResponse{}, err
	}
	proceeds, err := state.Proceeds.Coefficient()
	if err != nil {
		return gainImpactStateResponse{}, err
	}
	response := gainImpactStateResponse{AccountID: state.AccountID, CommodityID: state.CommodityID,
		CostCommodityID: state.CostCommodityID, DisposalDate: state.DisposalDate,
		CostBasisMethod: state.CostBasisMethod, QuantityValue: quantity, QuantityScale: state.Quantity.Scale(),
		BasisKnowledge: state.BasisKnowledge, ProceedsValue: proceeds, ProceedsScale: state.Proceeds.Scale()}
	if response.DisposedBasisValue, response.DisposedBasisScale, err = optionalCoefficient(state.DisposedBasis); err != nil {
		return gainImpactStateResponse{}, err
	}
	if response.RealizedGainValue, response.RealizedGainScale, err = optionalCoefficient(state.Gain); err != nil {
		return gainImpactStateResponse{}, err
	}
	return response, nil
}

func optionalCoefficient(value *exact.ScaledInt) (*exact.Coefficient, *int, error) {
	if value == nil {
		return nil, nil, nil
	}
	coefficient, err := value.Coefficient()
	if err != nil {
		return nil, nil, err
	}
	scale := value.Scale()
	return &coefficient, &scale, nil
}
