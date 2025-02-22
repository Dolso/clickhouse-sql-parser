package parser

import (
	"fmt"
	"strings"
)

func (p *Parser) tryParseColumnComment(pos Pos) (*StringLiteral, error) {
	if p.tryConsumeKeyword(KeywordComment) == nil {
		return nil, nil // nolint
	}
	return p.parseString(pos)
}

func (p *Parser) parseExpr(pos Pos) (Expr, error) {
	orExpr, err := p.parseOrExpr(pos)
	if err != nil {
		return orExpr, err
	}
	switch {
	case p.matchKeyword(KeywordAs): // syntax: columnExpr (alias | AS identifier)
		aliasPos := p.Pos()
		_ = p.lexer.consumeToken()
		asIdent, err := p.parseIdent()
		if err != nil {
			return nil, err
		}
		return &AliasExpr{
			AliasPos: aliasPos,
			Expr:     orExpr,
			Alias:    asIdent,
		}, nil
	}
	return orExpr, nil
}

func (p *Parser) parseOrExpr(pos Pos) (Expr, error) {
	expr, err := p.parseAndExpr(pos)
	if err != nil {
		return nil, err
	}
	for {
		if p.tryConsumeKeyword(KeywordOr) == nil {
			return expr, nil
		}

		rightExpr, err := p.parseAndExpr(p.Pos())
		if err != nil {
			return nil, err
		}
		expr = &BinaryExpr{
			LeftExpr:  expr,
			Operation: opTypeOr,
			RightExpr: rightExpr,
		}
	}
}

func (p *Parser) parseAndExpr(pos Pos) (Expr, error) {
	expr, err := p.parseNotExpr(pos)
	if err != nil {
		return nil, err
	}
	for {
		if p.tryConsumeKeyword(KeywordAnd) == nil {
			return expr, nil
		}

		rightExpr, err := p.parseNotExpr(p.Pos())
		if err != nil {
			return nil, err
		}
		expr = &BinaryExpr{
			LeftExpr:  expr,
			Operation: opTypeAnd,
			RightExpr: rightExpr,
		}
	}
}

func (p *Parser) parseNotExpr(pos Pos) (Expr, error) {
	if p.tryConsumeKeyword(KeywordNot) == nil {
		return p.parseIsOrNotNull(p.Pos())
	}

	notExpr, err := p.parseNotExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	return &NotExpr{
		NotPos: pos,
		Expr:   notExpr,
	}, nil
}

func (p *Parser) parseIsOrNotNull(pos Pos) (Expr, error) {
	expr, err := p.parseCompareExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	if p.tryConsumeKeyword(KeywordIs) == nil {
		return expr, nil
	}

	isNotNull := p.tryConsumeKeyword(KeywordNot) != nil
	if err := p.consumeKeyword(KeywordNull); err != nil {
		return nil, err
	}

	if isNotNull {
		return &IsNotNullExpr{
			IsPos: pos,
			Expr:  expr,
		}, nil
	}
	return &IsNullExpr{
		IsPos: pos,
		Expr:  expr,
	}, nil
}

func (p *Parser) parseCompareExpr(pos Pos) (Expr, error) {
	hasNot, hasGlobal := false, false
	expr, err := p.parseAddSubExpr(pos)
	if err != nil {
		return nil, err
	}
	switch {
	case p.matchTokenKind("["):
		params, err := p.parseArrayParams(pos)
		if err != nil {
			return nil, err
		}
		return &ObjectParams{
			Object: expr,
			Params: params,
		}, nil
	case p.matchTokenKind(opTypeEQ):
	case p.matchTokenKind(opTypeLT):
	case p.matchTokenKind(opTypeLE):
	case p.matchTokenKind(opTypeGE):
	case p.matchTokenKind(opTypeGT):
	case p.matchTokenKind(opTypeDoubleEQ):
	case p.matchTokenKind(opTypeNE):
	case p.matchTokenKind("<>"):
	case p.matchTokenKind(opTypeQuery):
	case p.matchKeyword(KeywordIn):
	case p.matchKeyword(KeywordLike):
	case p.matchKeyword(KeywordIlike):
	case p.matchKeyword(KeywordGlobal):
		_ = p.lexer.consumeToken()
		hasGlobal = true
	case p.matchKeyword(KeywordNot):
		_ = p.lexer.consumeToken()
		switch {
		case p.matchKeyword(KeywordIn):
		case p.matchKeyword(KeywordLike):
		case p.matchKeyword(KeywordIlike):
		default:
			return nil, fmt.Errorf("expected IN, LIKE or ILIKE after NOT, got %s", p.lastTokenKind())
		}
		hasNot = true
	default:
		return expr, nil
	}
	op := TokenKind(strings.ToUpper(p.last().String))
	_ = p.lexer.consumeToken()

	rightExpr, err := p.parseAddSubExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	return &BinaryExpr{
		LeftExpr:  expr,
		HasNot:    hasNot,
		HasGlobal: hasGlobal,
		Operation: op,
		RightExpr: rightExpr,
	}, nil
}

func (p *Parser) parseAddSubExpr(pos Pos) (Expr, error) {
	expr, err := p.parseMulDivModExpr(pos)
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case p.matchTokenKind(opTypePlus), p.matchTokenKind(opTypeMinus):
			op := p.lastTokenKind()
			_ = p.lexer.consumeToken()
			rightExpr, err := p.parseMulDivModExpr(p.Pos())
			if err != nil {
				return nil, err
			}
			expr = &BinaryExpr{
				LeftExpr:  expr,
				Operation: op,
				RightExpr: rightExpr,
			}
		default:
			return expr, nil
		}
	}
}

func (p *Parser) parseTernaryExpr(condition Expr) (*TernaryExpr, error) {
	if _, err := p.consumeTokenKind("?"); err != nil {
		return nil, err
	}
	trueExpr, err := p.parseExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	if _, err := p.consumeTokenKind(":"); err != nil {
		return nil, err
	}
	falseExpr, err := p.parseExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	return &TernaryExpr{
		Condition: condition,
		TrueExpr:  trueExpr,
		FalseExpr: falseExpr,
	}, nil
}

func (p *Parser) parseMulDivModExpr(pos Pos) (Expr, error) {
	expr, err := p.parseUnaryExpr(pos)
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case p.matchTokenKind(opTypeQuery):
			return p.parseTernaryExpr(expr)
		case p.matchTokenKind(opTypeMul),
			p.matchTokenKind(opTypeDiv),
			p.matchTokenKind(opTypeMod),
			p.matchTokenKind(TokenArrow),
			p.matchTokenKind(TokenCast):
			op := p.lastTokenKind()
			_ = p.lexer.consumeToken()
			rightExpr, err := p.parseUnaryExpr(p.Pos())
			if err != nil {
				return nil, err
			}
			expr = &BinaryExpr{
				LeftExpr:  expr,
				Operation: op,
				RightExpr: rightExpr,
			}
		default:
			return expr, nil
		}
	}
}

func (p *Parser) parseColumnExtractExpr(pos Pos) (*ExtractExpr, error) {
	if err := p.consumeKeyword(KeywordExtract); err != nil {
		return nil, err
	}
	if _, err := p.consumeTokenKind("("); err != nil {
		return nil, err
	}

	// parse interval
	ident, err := p.parseIdent()
	if err != nil {
		return nil, err
	}
	if !intervalType.Contains(strings.ToUpper(ident.Name)) {
		return nil, fmt.Errorf("unknown interval type: <%q>", ident.Name)
	}

	fromPos := p.Pos()
	if err := p.consumeKeyword(KeywordFrom); err != nil {
		return nil, err
	}

	expr, err := p.parseExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	if err := p.consumeKeyword(")"); err != nil {
		return nil, err
	}
	return &ExtractExpr{
		ExtractPos: pos,
		Interval:   ident,
		FromPos:    fromPos,
		FromExpr:   expr,
	}, nil
}

func (p *Parser) parseUnaryExpr(pos Pos) (Expr, error) {
	kind := p.last()
	switch {
	case p.matchTokenKind(opTypePlus),
		p.matchTokenKind(opTypeMinus),
		p.matchKeyword(KeywordNot):
		_ = p.lexer.consumeToken()
	default:
		return p.parseColumnExpr(pos)
	}

	var expr Expr
	var err error
	switch {
	case p.matchTokenKind(TokenIdent),
		p.matchTokenKind("("):
		expr, err = p.parseExpr(p.Pos())
	default:
		expr, err = p.parseColumnExpr(p.Pos())
	}
	if err != nil {
		return nil, err
	}

	return &UnaryExpr{
		UnaryPos: pos,
		Kind:     kind.Kind,
		Expr:     expr,
	}, nil

}

func (p *Parser) parseColumnExpr(pos Pos) (Expr, error) { //nolint:funlen
	switch {
	case p.matchKeyword(KeywordInterval):
		return p.parseColumnExprInterval(pos)
	case p.matchKeyword(KeywordDate), p.matchKeyword(KeywordTimestamp):
		nextToken, err := p.lexer.peekToken()
		if err != nil {
			return nil, err
		}
		if nextToken.Kind == TokenString {
			return p.parseString(pos)
		}
		return p.parseIdentOrFunction(pos)
	case p.matchKeyword(KeywordCast):
		return p.parseColumnCastExpr(pos)
	case p.matchKeyword(KeywordCase):
		return p.parseColumnCaseExpr(pos)
	case p.matchKeyword(KeywordExtract):
		return p.parseColumnExtractExpr(pos)
	case p.matchTokenKind(TokenIdent):
		return p.parseIdentOrFunction(pos)
	case p.matchTokenKind(TokenString): // string literal
		return p.parseString(pos)
	case p.matchTokenKind(TokenInt),
		p.matchTokenKind(TokenFloat): // number literal
		return p.parseNumber(pos)
	case p.matchTokenKind("("):
		if peek, _ := p.lexer.peekToken(); peek != nil {
			if peek.Kind == TokenKeyword && strings.EqualFold(peek.String, KeywordSelect) {
				return p.parseSelectQuery(pos)
			}
		}
		return p.parseFunctionParams(pos)
	case p.matchTokenKind("*"):
		return p.parseColumnStar(pos)
	case p.matchTokenKind("["):
		return p.parseArrayParams(pos)

	default:
		return nil, fmt.Errorf("unexpected token kind: %s", p.lastTokenKind())
	}
}

func (p *Parser) tryParseTableColumnPropertyExpr(pos Pos) (Expr, error) {
	switch {
	case p.matchKeyword(KeywordDefault):
		return p.parseDefaultExpr(pos)
	case p.matchKeyword(KeywordMaterialized):
	case p.matchKeyword(KeywordAlias):
	}
	return nil, nil // nolint
}

func (p *Parser) parseColumnCastExpr(pos Pos) (Expr, error) {
	if err := p.consumeKeyword(KeywordCast); err != nil {
		return nil, err
	}

	if _, err := p.consumeTokenKind("("); err != nil {
		return nil, err
	}

	columnExpr, err := p.parseExpr(p.Pos())
	if err != nil {
		return nil, err
	}

	asPos := p.Pos()
	if err := p.consumeKeyword(KeywordAs); err != nil {
		return nil, err
	}
	asColumnType, err := p.parseColumnType(p.Pos())
	if err != nil {
		return nil, err
	}

	if _, err := p.consumeTokenKind(")"); err != nil {
		return nil, err
	}

	return &CastExpr{
		CastPos: pos,
		AsPos:   asPos,
		Expr:    columnExpr,
		AsType:  asColumnType,
	}, nil
}

func (p *Parser) parseColumnExprListWithRoundBracket(pos Pos) (*ColumnExprList, error) {
	return p.parseColumnExprListWithTerm(")", pos)
}

func (p *Parser) parseColumnExprListWithSquareBracket(pos Pos) (*ColumnExprList, error) {
	return p.parseColumnExprListWithTerm("]", pos)
}

func (p *Parser) parseColumnExprList(pos Pos) (*ColumnExprList, error) {
	return p.parseColumnExprListWithTerm("", pos)
}

func (p *Parser) parseColumnExprListWithTerm(term TokenKind, pos Pos) (*ColumnExprList, error) {
	columnExprList := &ColumnExprList{
		ListPos: pos,
		ListEnd: pos,
	}
	columnExprList.HasDistinct = p.tryConsumeKeyword(KeywordDistinct) != nil
	columnList := make([]Expr, 0)
	for !p.lexer.isEOF() || p.last() != nil {
		if term != "" && p.matchTokenKind(term) {
			break
		}
		columnExpr, err := p.parseColumnsExpr(pos)
		if err != nil {
			return nil, err
		}
		if columnExpr == nil {
			break
		}
		columnList = append(columnList, columnExpr)
		if p.tryConsumeTokenKind(",") == nil {
			break
		}
	}
	columnExprList.Items = columnList
	if len(columnList) > 0 {
		columnExprList.ListEnd = columnList[len(columnList)-1].End()
	}
	return columnExprList, nil
}

// Syntax: INTERVAL expr interval
func (p *Parser) parseColumnExprInterval(pos Pos) (Expr, error) {
	if err := p.consumeKeyword(KeywordInterval); err != nil {
		return nil, err
	}

	// store the column expr if it needs
	columnExpr, err := p.parseExpr(p.Pos())
	if err != nil {
		return nil, err
	}

	// parse interval
	ident, err := p.parseIdent()
	if err != nil {
		return nil, err
	}
	if !intervalType.Contains(strings.ToUpper(ident.Name)) {
		return nil, fmt.Errorf("unknown interval type: <%q>", ident.Name)
	}
	return &IntervalExpr{
		IntervalPos: pos,
		Expr:        columnExpr,
		Unit:        ident,
	}, nil
}

func (p *Parser) parseFunctionExpr(_ Pos) (Expr, error) {
	if _, err := p.consumeTokenKind(TokenIdent); err != nil {
		return nil, err
	}
	// parse function name
	name, err := p.parseIdent()
	if err != nil {
		return nil, err
	}
	// parse function params
	params, err := p.parseFunctionParams(p.Pos())
	if err != nil {
		return nil, err
	}
	return &FunctionExpr{
		Name:   name,
		Params: params,
	}, nil
}

func (p *Parser) parseColumnArgList(pos Pos) (*ColumnArgList, error) {
	if _, err := p.consumeTokenKind("("); err != nil {
		return nil, err
	}
	distinct := false
	if p.tryConsumeKeyword(KeywordDistinct) != nil {
		distinct = true
	}
	var items []Expr
	for !p.lexer.isEOF() && !p.matchTokenKind(")") {
		item, err := p.parseExpr(p.Pos())
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		if p.tryConsumeTokenKind(",") == nil {
			break
		}
	}
	rightParenPos := p.Pos()
	if _, err := p.consumeTokenKind(")"); err != nil {
		return nil, err
	}
	return &ColumnArgList{
		LeftParenPos:  pos,
		RightParenPos: rightParenPos,
		Distinct:      distinct,
		Items:         items,
	}, nil
}

func (p *Parser) parseFunctionParams(pos Pos) (*ParamExprList, error) {
	if _, err := p.consumeTokenKind("("); err != nil {
		return nil, err
	}
	params, err := p.parseColumnExprListWithRoundBracket(p.Pos())
	if err != nil {
		return nil, err
	}
	rightParenPos := p.Pos()
	if _, err := p.consumeTokenKind(")"); err != nil {
		return nil, err
	}
	paramExprList := &ParamExprList{
		LeftParenPos:  pos,
		RightParenPos: rightParenPos,
		Items:         params,
	}

	// try to parse column arg list
	if p.matchTokenKind("(") {
		columnArgList, err := p.parseColumnArgList(p.Pos())
		if err != nil {
			return nil, err
		}
		paramExprList.ColumnArgList = columnArgList
	}
	return paramExprList, nil
}

func (p *Parser) parseArrayParams(pos Pos) (*ArrayParamList, error) {
	if _, err := p.consumeTokenKind("["); err != nil {
		return nil, err
	}
	params, err := p.parseColumnExprListWithSquareBracket(p.Pos())
	if err != nil {
		return nil, err
	}
	rightBracketPos := p.Pos()
	if _, err := p.consumeTokenKind("]"); err != nil {
		return nil, err
	}
	return &ArrayParamList{
		LeftBracketPos:  pos,
		RightBracketPos: rightBracketPos,
		Items:           params,
	}, nil
}

func (p *Parser) parseColumnsExpr(pos Pos) (Expr, error) {
	return p.parseExpr(pos)
}

func (p *Parser) parseColumnCaseExpr(pos Pos) (*CaseExpr, error) {
	// CASE expr
	caseExpr := &CaseExpr{CasePos: pos}
	if err := p.consumeKeyword(KeywordCase); err != nil {
		return nil, err
	}

	expr, err := p.parseExpr(p.Pos())
	if err != nil {
		return nil, err
	}
	caseExpr.Expr = expr

	// WHEN expr THEN expr
	whenExprs := make([]*WhenExpr, 0)
	for p.matchKeyword(KeywordWhen) {
		whenPos := p.Pos()
		_ = p.lexer.consumeToken()
		whenCondition, err := p.parseExpr(p.Pos())
		if err != nil {
			return nil, err
		}

		thenPos := p.Pos()
		if err := p.consumeKeyword(KeywordThen); err != nil {
			return nil, err
		}
		thenCondition, err := p.parseExpr(p.Pos())
		if err != nil {
			return nil, err
		}

		whenExprs = append(whenExprs, &WhenExpr{
			WhenPos: whenPos,
			ThenPos: thenPos,
			When:    whenCondition,
			Then:    thenCondition,
		})
	}
	caseExpr.Whens = whenExprs

	// ELSE expr
	if elseToken := p.tryConsumeKeyword(KeywordElse); elseToken != nil {
		elseExpr, err := p.parseExpr(p.Pos())
		if err != nil {
			return nil, err
		}
		caseExpr.ElsePos = elseToken.Pos
		caseExpr.Else = elseExpr
	}

	if err := p.consumeKeyword(KeywordEnd); err != nil {
		return nil, err
	}

	return caseExpr, nil
}

func (p *Parser) parseColumnType(_ Pos) (Expr, error) { // nolint:funlen
	ident, err := p.parseIdent()
	if err != nil {
		return nil, err
	}
	if p.tryConsumeTokenKind("(") != nil {
		switch {
		case p.matchTokenKind(TokenIdent):
			if ident.Name == "Nested" {
				return p.parseNestedType(ident, p.Pos())
			}
			return p.parseComplexType(ident, p.Pos())
		case p.matchTokenKind(TokenString):
			if peekToken, err := p.lexer.peekToken(); err == nil && peekToken.Kind == "=" {
				// enum values
				return p.parseEnumExpr(p.Pos())
			}
			// like Datetime('Asia/Dubai')
			return p.parseColumnTypeWithParams(ident, p.Pos())
		case p.matchTokenKind(TokenInt):
			// fixed size
			return p.parseColumnTypeWithParams(ident, p.Pos())
		default:
			return nil, fmt.Errorf("unexpected token kind: %v", p.lastTokenKind())
		}
	}
	return &ScalarTypeExpr{Name: ident}, nil
}

func (p *Parser) parseColumnPropertyType(_ Pos) (Expr, error) {
	ident, err := p.parseIdent()
	if err != nil {
		return nil, err
	}
	return &PropertyTypeExpr{
		Name: ident,
	}, nil
}

func (p *Parser) parseComplexType(name *Ident, pos Pos) (Expr, error) {
	subTypes := make([]Expr, 0)
	for !p.lexer.isEOF() && !p.matchTokenKind(")") {
		subExpr, err := p.parseColumnType(p.Pos())
		if err != nil {
			return nil, err
		}
		subTypes = append(subTypes, subExpr)
		if p.tryConsumeTokenKind(",") == nil {
			break
		}
	}
	rightParenPos := p.Pos()
	if _, err := p.consumeTokenKind(")"); err != nil {
		return nil, err
	}
	return &ComplexTypeExpr{
		LeftParenPos:  pos,
		RightParenPos: rightParenPos,
		Name:          name,
		Params:        subTypes,
	}, nil
}

func (p *Parser) parseEnumExpr(pos Pos) (*EnumValueExprList, error) {
	expr := &EnumValueExprList{
		ListPos: pos,
		Enums:   make([]EnumValueExpr, 0),
	}
	for !p.lexer.isEOF() && !p.matchTokenKind(")") {
		enumValueExpr, err := p.parseEnumValueExpr(p.Pos())
		if err != nil {
			return nil, err
		}
		if enumValueExpr == nil {
			break
		}
		expr.Enums = append(expr.Enums, *enumValueExpr)
		if p.tryConsumeTokenKind(",") == nil {
			break
		}
	}
	if len(expr.Enums) > 0 {
		expr.ListEnd = expr.Enums[len(expr.Enums)-1].Value.NumEnd
	}
	if _, err := p.consumeTokenKind(")"); err != nil {
		return nil, err
	}
	return expr, nil
}

func (p *Parser) parseColumnTypeWithParams(name *Ident, pos Pos) (*TypeWithParamsExpr, error) {
	params := make([]Literal, 0)
	param, err := p.parseLiteral(pos)
	if err != nil {
		return nil, err
	}
	params = append(params, param)
	for !p.lexer.isEOF() && p.tryConsumeTokenKind(",") != nil {
		size, err := p.parseLiteral(p.Pos())
		if err != nil {
			return nil, err
		}
		params = append(params, size)
	}

	rightParenPos := p.Pos()
	if _, err := p.consumeTokenKind(")"); err != nil {
		return nil, err
	}
	return &TypeWithParamsExpr{
		Name:          name,
		LeftParenPos:  pos,
		RightParenPos: rightParenPos,
		Params:        params,
	}, nil
}

func (p *Parser) parseNestedType(name *Ident, pos Pos) (*NestedTypeExpr, error) {
	columns, err := p.parseTableColumns()
	if err != nil {
		return nil, err
	}
	rightParenPos := p.Pos()
	if _, err := p.consumeTokenKind(")"); err != nil {
		return nil, err
	}
	return &NestedTypeExpr{
		LeftParenPos:  pos,
		RightParenPos: rightParenPos,
		Name:          name,
		Columns:       columns,
	}, nil
}

func (p *Parser) tryParseCompressionCodecs(pos Pos) (*CompressionCodec, error) {
	if p.tryConsumeKeyword(KeywordCodec) == nil {
		return nil, nil // nolint
	}

	if _, err := p.consumeTokenKind("("); err != nil {
		return nil, err
	}

	// parse codec name
	name, err := p.parseIdent()
	if err != nil {
		return nil, err
	}
	var level *NumberLiteral
	// TODO: check if the codec name is valid
	switch strings.ToUpper(name.Name) {
	case "ZSTD", "LZ4HC":
		level, err = p.tryParseCompressionLevel(p.Pos())
		if err != nil {
			return nil, err
		}
	}

	rightParenPos := p.last().End
	if _, err := p.consumeTokenKind(")"); err != nil {
		return nil, err
	}

	return &CompressionCodec{
		CodecPos:      pos,
		RightParenPos: rightParenPos,
		Name:          name,
		Level:         level,
	}, nil
}

func (p *Parser) parseEnumValueExpr(pos Pos) (*EnumValueExpr, error) {
	if _, err := p.consumeTokenKind(TokenString); err != nil {
		return nil, err
	}
	name, err := p.parseString(pos)
	if err != nil {
		return nil, err
	}

	if _, err := p.consumeTokenKind("="); err != nil {
		return nil, err
	}

	value, err := p.parseNumber(p.Pos())
	if err != nil {
		return nil, err
	}
	return &EnumValueExpr{
		Name:  name,
		Value: value,
	}, nil
}

func (p *Parser) parseColumnStar(pos Pos) (*Ident, error) {
	if _, err := p.consumeTokenKind("*"); err != nil {
		return nil, err
	}
	return &Ident{
		NamePos: pos,
		NameEnd: pos,
		Name:    "*",
	}, nil
}

func (p *Parser) tryParseCompressionLevel(pos Pos) (*NumberLiteral, error) {
	if p.tryConsumeTokenKind("(") == nil {
		return nil, nil // nolint
	}

	num, err := p.parseNumber(pos)
	if err != nil {
		return nil, err
	}

	if _, err := p.consumeTokenKind(")"); err != nil {
		return nil, err
	}
	return num, nil
}
