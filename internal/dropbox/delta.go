package dropbox

import "context"

type DeltaPageFetcher interface {
	ListFolder(ctx context.Context, path string, recursive bool) (*ListFolderResult, error)
	ListFolderContinue(ctx context.Context, cursor string) (*ListFolderResult, error)
}

type DeltaResult struct {
	Entries []Metadata
	Cursor  string
	Pages   int
}

func FetchDelta(ctx context.Context, client DeltaPageFetcher, cursor, path string, recursive bool) (DeltaResult, error) {
	var page *ListFolderResult
	var err error
	if cursor == "" {
		page, err = client.ListFolder(ctx, path, recursive)
	} else {
		page, err = client.ListFolderContinue(ctx, cursor)
	}
	if err != nil {
		return DeltaResult{}, err
	}
	out := DeltaResult{Entries: append([]Metadata{}, page.Entries...), Cursor: page.Cursor, Pages: 1}
	for page.HasMore {
		page, err = client.ListFolderContinue(ctx, out.Cursor)
		if err != nil {
			return out, err
		}
		out.Entries = append(out.Entries, page.Entries...)
		out.Cursor = page.Cursor
		out.Pages++
	}
	return out, nil
}
