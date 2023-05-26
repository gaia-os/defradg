// Copyright 2022 Democratized Data Foundation
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package one_to_many_to_one

import (
	"testing"

	testUtils "github.com/sourcenetwork/defradb/tests/integration"
)

func TestQueryComplexWithDeepFilterOnRenderedChildren(t *testing.T) {
	test := testUtils.TestCase{
		Description: "One-to-many-to-one deep filter on rendered children.",
		Actions: []any{
			gqlSchemaOneToManyToOne(),
			// Authors
			testUtils.CreateDoc{
				CollectionID: 0,
				// bae-41598f0c-19bc-5da6-813b-e80f14a10df3, Has written 5 books
				Doc: `{
					"name": "John Grisham",
					"age": 65,
					"verified": true
				}`,
			},
			testUtils.CreateDoc{
				CollectionID: 0,
				// bae-b769708d-f552-5c3d-a402-ccfd7ac7fb04, Has written 1 Book
				Doc: `{
					"name": "Cornelia Funke",
					"age": 62,
					"verified": false
				}`,
			},
			testUtils.CreateDoc{
				CollectionID: 0,
				// Has written no Book
				Doc: `{
					"name": "Not a Writer",
					"age": 6,
					"verified": false
				}`,
			},
			// Books
			testUtils.CreateDoc{
				CollectionID: 1,
				// "bae-b6c078f2-3427-5b99-bafd-97dcd7c2e935", Has 1 Publisher
				Doc: `{
					"name": "The Rooster Bar",
					"rating": 4,
					"author_id": "bae-b769708d-f552-5c3d-a402-ccfd7ac7fb04"
				}`,
			},
			testUtils.CreateDoc{
				CollectionID: 1,
				// "bae-b8091c4f-7594-5d7a-98e8-272aadcedfdf", Has 1 Publisher
				Doc: `{
					"name": "Theif Lord",
					"rating": 4.8,
					"author_id": "bae-41598f0c-19bc-5da6-813b-e80f14a10df3"
				}`,
			},
			testUtils.CreateDoc{
				CollectionID: 1,
				// "bae-4fb9e3e9-d1d3-5404-bf15-10e4c995d9ca", Has no Publisher.
				Doc: `{
					"name": "The Associate",
					"rating": 4.2,
					"author_id": "bae-41598f0c-19bc-5da6-813b-e80f14a10df3"
				}`,
			},
			// Publishers
			testUtils.CreateDoc{
				CollectionID: 2,
				Doc: `{
					"name": "Only Publisher of The Rooster Bar",
					"address": "1 Rooster Ave., Waterloo, Ontario",
					"yearOpened": 2022,
					"book_id": "bae-b6c078f2-3427-5b99-bafd-97dcd7c2e935"
			    }`,
			},
			testUtils.CreateDoc{
				CollectionID: 2,
				Doc: `{
					"name": "Only Publisher of Theif Lord",
					"address": "1 Theif Lord, Waterloo, Ontario",
					"yearOpened": 2020,
					"book_id": "bae-b8091c4f-7594-5d7a-98e8-272aadcedfdf"
			    }`,
			},
			testUtils.Request{
				Request: `query {
					Author (filter: {book: {publisher: {yearOpened: {_gt: 2021}}}}) {
						name
						book {
							publisher {
								yearOpened
							}
						}
					}
				}`,
				Results: []map[string]any{
					{
						"name": "Cornelia Funke",
						"book": []map[string]any{
							{
								"publisher": map[string]any{
									"yearOpened": uint64(2022),
								},
							},
						},
					},
				},
			},
		},
	}

	testUtils.ExecuteTestCase(t, []string{"Author", "Book", "Publisher"}, test)
}

func TestOneToManyToOneWithSumOfDeepFilterSubTypeOfBothDescAndAsc(t *testing.T) {
	test := testUtils.TestCase{
		Description: "1-N-1 sums of deep filter subtypes of both descending and ascending.",
		Actions: []any{
			gqlSchemaOneToManyToOne(),
			createDocsWith6BooksAnd5Publishers(),
			testUtils.Request{
				Request: `query {
					Author {
						name
						s1: _sum(book: {field: rating, filter: {publisher: {yearOpened: {_eq: 2013}}}})
						s2: _sum(book: {field: rating, filter: {publisher: {yearOpened: {_ge: 2020}}}})
					}
				}`,
				Results: []map[string]any{
					{
						"name": "John Grisham",
						// 'Theif Lord' (4.8 rating) 2020, then 'A Time for Mercy' 2013 (4.5 rating).
						"s1": 4.5,
						// 'The Associate' as it has no Publisher (4.2 rating), then 'Painted House' 1995 (4.9 rating).
						"s2": 4.8,
					},
					{
						"name": "Not a Writer",
						"s1":   0.0,
						"s2":   0.0,
					},
					{
						"name": "Cornelia Funke",
						"s1":   0.0,
						"s2":   4.0,
					},
				},
			},
		},
	}

	testUtils.ExecuteTestCase(t, []string{"Author", "Book", "Publisher"}, test)
}

func TestOneToManyToOneWithSumOfDeepFilterSubTypeAndDeepOrderBySubtypeOppositeDirections(t *testing.T) {
	test := testUtils.TestCase{
		Description: "1-N-1 sum of deep filter subtypes and non-sum deep filter",
		Actions: []any{
			gqlSchemaOneToManyToOne(),
			createDocsWith6BooksAnd5Publishers(),
			testUtils.Request{
				Request: `query {
					Author {
						name
						s1: _sum(book: {field: rating, filter: {publisher: {yearOpened: {_eq: 2013}}}})
						books2020: book(filter: {publisher: {yearOpened: {_ge: 2020}}}) {
							name
						}
					}
				}`,
				Results: []map[string]any{
					{
						"name": "John Grisham",
						"s1":   4.5,
						"books2020": []map[string]any{
							{
								"name": "Theif Lord",
							},
						},
					},
					{
						"name":      "Not a Writer",
						"s1":        0.0,
						"books2020": []map[string]any{},
					},
					{
						"name": "Cornelia Funke",
						"s1":   0.0,
						"books2020": []map[string]any{
							{
								"name": "The Rooster Bar",
							},
						},
					},
				},
			},
		},
	}

	testUtils.ExecuteTestCase(t, []string{"Author", "Book", "Publisher"}, test)
}

func TestOneToManyToOneWithTwoLevelDeepFilter(t *testing.T) {
	test := testUtils.TestCase{
		Description: "1-N-1 two level deep filter",
		Actions: []any{
			gqlSchemaOneToManyToOne(),
			createDocsWith6BooksAnd5Publishers(),
			testUtils.Request{
				Request: `query {
					Author (filter: {book: {publisher: {yearOpened: { _ge: 2020}}}}){
						name
						book {
							name
							publisher {
								yearOpened
							}
						}
					}
				}`,
				Results: []map[string]any{
					{
						"book": []map[string]any{
							{
								"name":      "The Associate",
								"publisher": nil,
							},
							{
								"name": "Sooley",
								"publisher": map[string]any{
									"yearOpened": uint64(1999),
								},
							},
							{
								"name": "Theif Lord",
								"publisher": map[string]any{
									"yearOpened": uint64(2020),
								},
							},
							{
								"name": "Painted House",
								"publisher": map[string]any{
									"yearOpened": uint64(1995),
								},
							},
							{
								"name": "A Time for Mercy",
								"publisher": map[string]any{
									"yearOpened": uint64(2013),
								},
							},
						},
						"name": "John Grisham",
					},
					{
						"book": []map[string]any{
							{
								"name": "The Rooster Bar",
								"publisher": map[string]any{
									"yearOpened": uint64(2022),
								},
							},
						},
						"name": "Cornelia Funke",
					},
				},
			},
		},
	}

	testUtils.ExecuteTestCase(t, []string{"Author", "Book", "Publisher"}, test)
}
