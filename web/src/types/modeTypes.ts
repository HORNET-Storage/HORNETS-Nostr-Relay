// Define the state and action types for mode settings
export interface ModeState {
    relayMode: 'unlimited' | 'smart';
    kinds: number[];
    mediaTypes: string[];
  }
  
  export interface ModeAction {
    type: string;
    payload: any;
  }
  