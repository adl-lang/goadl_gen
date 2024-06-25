package gengo

func (in *GenGo) Run() error {
	if err := in.GoTypes.Run(); err != nil {
		return err
	}
	return nil
}
