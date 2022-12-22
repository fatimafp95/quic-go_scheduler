package quic

import (
	"errors"
	"fmt"
	"github.com/lucas-clemente/quic-go/internal/ackhandler"
	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/wire"
	"github.com/lucas-clemente/quic-go/quicvarint"
	"sync"
)

type framer interface {
	HasData() bool

	QueueControlFrame(wire.Frame)
	AppendControlFrames([]ackhandler.Frame, protocol.ByteCount) ([]ackhandler.Frame, protocol.ByteCount)

	AddActiveStream(protocol.StreamID)
	AppendStreamFrames([]ackhandler.Frame, protocol.ByteCount) ([]ackhandler.Frame, protocol.ByteCount)

	Handle0RTTRejection() error

	HasRetransmission() bool
}

type framerI struct {
	mutex sync.Mutex

	streamGetter streamGetter
	version      protocol.VersionNumber

	activeStreams map[protocol.StreamID]struct{}
	streamQueue   []protocol.StreamID

	controlFrameMutex sync.Mutex
	controlFrames  []wire.Frame
	config         *Config
	streamMapPrior map[protocol.StreamID]int //To match priorities with the stream ID
	auxPriorSlice []int
}

var _ framer = &framerI{}

func newFramer(
	streamGetter streamGetter,
	v protocol.VersionNumber,
	config *Config,
) framer {
	return &framerI{
		streamGetter:  streamGetter,
		activeStreams: make(map[protocol.StreamID]struct{}),
		version:       v,
		streamMapPrior: make(map[protocol.StreamID]int),
		config:		config,
	}
}

func (f *framerI) HasData() bool {
	f.mutex.Lock()
	hasData := len(f.streamQueue) > 0
	f.mutex.Unlock()
	if hasData {
		return true
	}
	f.controlFrameMutex.Lock()
	hasData = len(f.controlFrames) > 0
	f.controlFrameMutex.Unlock()
	return hasData
}

func (f *framerI) QueueControlFrame(frame wire.Frame) {
	f.controlFrameMutex.Lock()
	f.controlFrames = append(f.controlFrames, frame)
	f.controlFrameMutex.Unlock()
}

func (f *framerI) AppendControlFrames(frames []ackhandler.Frame, maxLen protocol.ByteCount) ([]ackhandler.Frame, protocol.ByteCount) {
	var length protocol.ByteCount
	f.controlFrameMutex.Lock()
	for len(f.controlFrames) > 0 {
		frame := f.controlFrames[len(f.controlFrames)-1]
		frameLen := frame.Length(f.version)
		if length+frameLen > maxLen {
			break
		}
		frames = append(frames, ackhandler.Frame{Frame: frame})
		length += frameLen
		f.controlFrames = f.controlFrames[:len(f.controlFrames)-1]
	}
	f.controlFrameMutex.Unlock()
	return frames, length
}

func (f *framerI) AddActiveStream(id protocol.StreamID) {

	f.mutex.Lock()
	defer f.mutex.Unlock()
	if _, ok := f.activeStreams[id]; !ok {
		f.streamQueue = append(f.streamQueue, id)
		f.activeStreams[id] = struct{}{}

		fmt.Println("AddActiveStream Streamqueue UPD: ", f.streamQueue, id)
		fmt.Println("AddActiveStream StreamPrior: ", f.config.StreamPrior)

		switch f.config.TypePrior {
		case "fifo":
			fmt.Println("\nEstás usando FIFO\n")
		case "abs"://The stream queue is ordered by StreamPrior priorities slice.

			lenQ := len(f.streamQueue)
			prior := 1
			fmt.Println("Inicio del case de abs el valor de la streamqueue es: \n",f.streamQueue, lenQ, prior,f.config.StreamPrior)
			//To assign priority to each slice in a map
			if v, ok := f.streamMapPrior[id]; ok {
				prior=v
			}else{
				fmt.Println("Else: \n",f.streamQueue, lenQ, prior,f.config.StreamPrior)
				if len(f.config.StreamPrior) > 0 {
					prior = f.config.StreamPrior[0]
					fmt.Println("Inicio del if len(f.config.Streamprior>0): \n",f.streamQueue, lenQ, prior,f.config.StreamPrior)
					fmt.Println("Valor de auxPriorSlice: \n",f.auxPriorSlice)
					f.config.StreamPrior = f.config.StreamPrior[1:] //Delete the used priority for the next stream
					fmt.Println("Final del if len(f.config.Streamprior>0): \n",f.streamQueue, lenQ, prior,f.config.StreamPrior, f.auxPriorSlice)
				}
				f.streamMapPrior[id] = prior ///To assign priority to each slice in a map
			}

			f.auxPriorSlice = append(f.auxPriorSlice,prior)
			fmt.Println(f.streamMapPrior)

			//Absolute priorization: the stream queue is ordered regarding the priorities of the stream (StreamPrior slice)
			//which is also ordered
			fmt.Println(">>> order find-", f.streamQueue, f.auxPriorSlice)
			posIni := lenQ-1
			newPrior := f.auxPriorSlice[posIni]
			var correctPos int //Correct position of the stream/prior regarding the prior
			for i := lenQ-1; i >= 0 ; i--{
				if  newPrior >= f.auxPriorSlice[i] {
					correctPos=i
				}
			}
			//To insert the stream ID and priority in the correct position
			fmt.Println(">>> order start", f.streamQueue, f.auxPriorSlice, correctPos)
			auxSlice := append(f.auxPriorSlice[:correctPos], append([]int{newPrior}, f.auxPriorSlice[correctPos:posIni]...)...)
			copy(f.auxPriorSlice, auxSlice)

			//f.config.StreamPrior = append(f.config.StreamPrior[:correctPos],append([]int{newPrior},f.config.StreamPrior[posIni:]...)...) //para quitarlo de la config original

			f.streamQueue = append(f.streamQueue[:correctPos], append([]protocol.StreamID{id}, f.streamQueue[correctPos:posIni]...)...)
			// slice[:correctPos] + [5] + slice[initPos:]
			fmt.Println(">>> order -end-", f.streamQueue, f.auxPriorSlice)


		//Weighted fair queueing: the stream IDs are repeated in the stream queue regarding its priority
		case "wfq":
			fmt.Println("\nEstás usando wfq\n")
			fmt.Println("Inicio del case de wfq el valor de la streamqueue es: %v",f.streamQueue)

			prior := 1

			if v, ok := f.streamMapPrior[id]; ok {
				prior = v
			} else {
				//If there is priorities in the StreamPrior slice, pick the first one which corresponds to the current stream:
				if len(f.config.StreamPrior) > 0 {
					prior = f.config.StreamPrior[0]
					f.config.StreamPrior = f.config.StreamPrior[1:] //Delete the used priority for the next stream
				}
				f.streamMapPrior[id] = prior ///To assign priority to each slice in a map
				fmt.Println("Asignación de prioridades a los streams, estado de la streamqueue es: %v",f.streamQueue)

			}
			fmt.Println(f.streamMapPrior)
			//Stream ID is replicated in the streamQueue
			fmt.Println("WFQ: Replicando los IDs")
			for m:= 0; m<prior-1; m++ {
				f.streamQueue = append(f.streamQueue, id)
			}
			fmt.Println("%v",f.streamQueue)

		case "rr": // stream ID has already been added
			fmt.Println("\nEstás usando RR\n")

		default: // RR, already done ;)
		}
	//f.mutex.Unlock()
	}

}

func (f *framerI) AppendStreamFrames(frames []ackhandler.Frame, maxLen protocol.ByteCount) ([]ackhandler.Frame, protocol.ByteCount) {
	var length protocol.ByteCount
	var lastFrame *ackhandler.Frame
	f.mutex.Lock()

	// pop STREAM frames, until less than MinStreamFrameSize bytes are left in the packet
	numActiveStreams := len(f.streamQueue)
	for i := 0; i < numActiveStreams; i++ {
		if protocol.MinStreamFrameSize+length > maxLen || i >= len(f.streamQueue) {
			break
		}
		id := f.streamQueue[0]
		f.streamQueue = f.streamQueue[1:]

		// This should never return an error. Better check it anyway.
		// The stream will only be in the streamQueue, if it enqueued itself there.
		str, err := f.streamGetter.GetOrOpenSendStream(id)
		// The stream can be nil if it completed after it said it had data.
		if str == nil || err != nil {
			delete(f.activeStreams, id)
			f.CleanStreamQueueWFQ(id)
			if f.config.TypePrior == "abs" {
				f.config.StreamPrior = f.config.StreamPrior[1:]
			}
			continue
		}
		remainingLen := maxLen - length
		// For the last STREAM frame, we'll remove the DataLen field later.
		// Therefore, we can pretend to have more bytes available when popping
		// the STREAM frame (which will always have the DataLen set).
		remainingLen += quicvarint.Len(uint64(remainingLen))
		frame, hasMoreData := str.popStreamFrame(remainingLen)

		if f.config.TypePrior == "abs" {
			//fmt.Println("Estás usando el ABS")
			if hasMoreData { // put the stream front in the queue (at the beginning)
				f.streamQueue = append([]protocol.StreamID{id},f.streamQueue...)
			} else { // no more data to send. Stream is not active anymore
				delete(f.activeStreams, id)
				//Delete the priority of the stream in order not to confuse priorities with the arrival of new streams
				f.auxPriorSlice=f.auxPriorSlice[1:]
			}
		}else{ //WFQ or RR
			if hasMoreData { // put the stream back in the queue (at the end)
				f.streamQueue = append(f.streamQueue, id)
			} else { // no more data to send. Stream is not active anymore
				delete(f.activeStreams, id)
				//Delete the ID of the replicated stream
				f.CleanStreamQueueWFQ(id)
			}
		}

		// The frame can be nil
		// * if the receiveStream was canceled after it said it had data
		// * the remaining size doesn't allow us to add another STREAM frame
		if frame == nil {
			continue
		}
		frames = append(frames, *frame)
		length += frame.Length(f.version)
		lastFrame = frame
	}
	f.mutex.Unlock()
	if lastFrame != nil {
		lastFrameLen := lastFrame.Length(f.version)
		// account for the smaller size of the last STREAM frame
		lastFrame.Frame.(*wire.StreamFrame).DataLenPresent = false
		length += lastFrame.Length(f.version) - lastFrameLen
	}

	return frames, length
}

func (f *framerI) CleanStreamQueueWFQ(id protocol.StreamID){
	if f.config.TypePrior == "wfq"{
		fmt.Printf("\nEstás en CleanStreamQueueWFQ\n")
		for i := len(f.streamQueue)-1; i >= 0; i-- {
			fmt.Println(">>>", f.streamQueue, i)
			if f.streamQueue[i] == id {
				if i == len(f.streamQueue)-1 {
					fmt.Println("    >>> i -> last")
					f.streamQueue = f.streamQueue[:i]
					fmt.Println("Streamqueue:",f.streamQueue)
				} else {
					fmt.Println("    >>> i -> not last")
					f.streamQueue = append(f.streamQueue[:i], f.streamQueue[i+1:]...)
				}
			}
		}
		fmt.Println(">>> goodbye for")
	}
}

func (f *framerI) Handle0RTTRejection() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.controlFrameMutex.Lock()
	f.streamQueue = f.streamQueue[:0]
	for id := range f.activeStreams {
		delete(f.activeStreams, id)
	}
	var j int
	for i, frame := range f.controlFrames {
		switch frame.(type) {
		case *wire.MaxDataFrame, *wire.MaxStreamDataFrame, *wire.MaxStreamsFrame:
			return errors.New("didn't expect MAX_DATA / MAX_STREAM_DATA / MAX_STREAMS frame to be sent in 0-RTT")
		case *wire.DataBlockedFrame, *wire.StreamDataBlockedFrame, *wire.StreamsBlockedFrame:
			continue
		default:
			f.controlFrames[j] = f.controlFrames[i]
			j++
		}
	}
	f.controlFrames = f.controlFrames[:j]
	f.controlFrameMutex.Unlock()
	return nil
}

// HasRetransmission checks if retransmission queue is empty
// this check is necessary for Delivery Rate Estimation
func (f *framerI) HasRetransmission() bool {
	return f.streamGetter.HasRetransmission()
}
