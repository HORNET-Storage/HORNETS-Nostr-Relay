import { createSlice, PayloadAction } from '@reduxjs/toolkit';
import { ModeState } from '@app/types/modeTypes';

const initialState: ModeState = {
  relayMode: 'unlimited',
  kinds: [],
  mediaTypes: [],
};

export const modeSlice = createSlice({
  name: 'relaymode',
  initialState,
  reducers: {
    setMode: (state, action: PayloadAction<'unlimited' | 'smart'>) => {
      console.log(`Before update: ${state.relayMode}`);
      state.relayMode = action.payload;
      console.log(`After update: ${state.relayMode}`);
      // state.relayMode = action.payload;
      // // Automatically reset kinds and media types if mode is switched to 'base'
      // if (action.payload === 'unlimited') {
      //   state.kinds = [];
      //   state.mediaTypes = [];
      // }
    },
    setKinds: (state, action: PayloadAction<number[]>) => {
      if (state.relayMode === 'smart') {
        state.kinds = action.payload;
      }
    },
    setMediaTypes: (state, action: PayloadAction<string[]>) => {
      if (state.relayMode === 'smart') {
        state.mediaTypes = action.payload;
      }
    },
  },
});

export const { setMode, setKinds, setMediaTypes } = modeSlice.actions;

export default modeSlice.reducer;

