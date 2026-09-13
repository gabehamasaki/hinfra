package actions

import (
	"fmt"

	docstore "github.com/gabehamasaki/hinfra/tools/hinfra/internal/docs"
)

type InfraDocsResult struct {
	Content string                  `json:"content,omitempty"`
	Results []docstore.SearchResult `json:"results,omitempty"`
}

func InfraDocs(env *Env, search, read string) (InfraDocsResult, error) {
	store := docstore.NewStore(env.Runtime.InfraRepo)
	if read != "" {
		content, err := store.Read(read)
		if err != nil {
			return InfraDocsResult{}, err
		}
		return InfraDocsResult{Content: content}, nil
	}
	if search != "" {
		results, err := store.Search(search)
		if err != nil {
			return InfraDocsResult{}, err
		}
		return InfraDocsResult{Results: results}, nil
	}
	return InfraDocsResult{}, fmt.Errorf("informe search ou read")
}
