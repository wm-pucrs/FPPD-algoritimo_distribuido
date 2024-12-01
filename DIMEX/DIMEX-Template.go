/*  Construido como parte da disciplina: FPPD - PUCRS - Escola Politecnica
    Professor: Fernando Dotti  (https://fldotti.github.io/)
    Modulo representando Algoritmo de Exclusão Mútua Distribuída:
    Semestre 2023/1
	Aspectos a observar:
	   mapeamento de módulo para estrutura
	   inicializacao
	   semantica de concorrência: cada evento é atômico
	   							  módulo trata 1 por vez
	Q U E S T A O
	   Além de obviamente entender a estrutura ...
	   Implementar o núcleo do algoritmo ja descrito, ou seja, o corpo das
	   funcoes reativas a cada entrada possível:
	   			handleUponReqEntry()  // recebe do nivel de cima (app)
				handleUponReqExit()   // recebe do nivel de cima (app)
				handleUponDeliverRespOk(msgOutro)   // recebe do nivel de baixo
				handleUponDeliverReqEntry(msgOutro) // recebe do nivel de baixo
*/

package DIMEX

import (
	PP2PLink "SD/PP2PLink"
	"fmt"
	"strconv"
	"strings"
)

// ------------------------------------------------------------------------------------
// ------- principais tipos
// ------------------------------------------------------------------------------------

type State int // enumeracao dos estados possiveis de um processo
const (
	noMX State = iota
	wantMX
	inMX
)

type dmxReq int // enumeracao dos estados possiveis de um processo
const (
	ENTER dmxReq = iota
	EXIT
)

type dmxResp struct { // mensagem do módulo DIMEX infrmando que pode acessar - pode ser somente um sinal (vazio)
	// mensagem para aplicacao indicando que pode prosseguir
}

func customMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type DIMEX_Module struct {
	Req       chan dmxReq  // canal para receber pedidos da aplicacao (REQ e EXIT)
	Ind       chan dmxResp // canal para informar aplicacao que pode acessar
	processes []string     // endereco de todos, na mesma ordem
	id        int          // identificador do processo - é o indice no array de enderecos acima
	st        State        // estado deste processo na exclusao mutua distribuida
	waiting   []bool       // processos aguardando tem flag true
	lcl       int          // relogio logico local
	reqTs     int          // timestamp local da ultima requisicao deste processo
	nbrResps  int
	dbg       bool

	Pp2plink *PP2PLink.PP2PLink // acesso aa comunicacao enviar por PP2PLinq.Req  e receber por PP2PLinq.Ind
}

// ------------------------------------------------------------------------------------
// ------- inicializacao
// ------------------------------------------------------------------------------------

func NewDIMEX(_addresses []string, _id int, _dbg bool) *DIMEX_Module {

	p2p := PP2PLink.NewPP2PLink(_addresses[_id], _dbg)

	dmx := &DIMEX_Module{
		Req: make(chan dmxReq, 1),
		Ind: make(chan dmxResp, 1),

		processes: _addresses,
		id:        _id,
		st:        noMX,
		waiting:   make([]bool, len(_addresses)),
		lcl:       0,
		reqTs:     0,
		dbg:       _dbg,

		Pp2plink: p2p}

	for i := 0; i < len(dmx.waiting); i++ {
		dmx.waiting[i] = false
	}
	dmx.Start()
	dmx.outDbg("Init DIMEX!")
	return dmx
}

// ------------------------------------------------------------------------------------
// ------- nucleo do funcionamento
// ------------------------------------------------------------------------------------

func (module *DIMEX_Module) Start() {

	go func() {
		for {
			select {
			case dmxR := <-module.Req: // vindo da  aplicação
				if dmxR == ENTER {
					module.outDbg("app pede mx")
					module.handleUponReqEntry() // ENTRADA DO ALGORITMO

				} else if dmxR == EXIT {
					module.outDbg("app libera mx")
					module.handleUponReqExit() // ENTRADA DO ALGORITMO
				}

			case msgOutro := <-module.Pp2plink.Ind: // vindo de outro processo
				//fmt.Printf("dimex recebe da rede: ", msgOutro)
				if strings.Contains(msgOutro.Message, "respOK") {
					module.outDbg("         <<<---- responde! " + msgOutro.Message)
					module.handleUponDeliverRespOk(msgOutro) // ENTRADA DO ALGORITMO

				} else if strings.Contains(msgOutro.Message, "reqEntry") {
					module.outDbg("          <<<---- pede??  " + msgOutro.Message)
					module.handleUponDeliverReqEntry(msgOutro) // ENTRADA DO ALGORITMO

				}
			}
		}
	}()
}

// ------------------------------------------------------------------------------------
// ------- tratamento de pedidos vindos da aplicacao
// ------- UPON ENTRY
// ------- UPON EXIT
// ------------------------------------------------------------------------------------

func (module *DIMEX_Module) handleUponReqEntry() {
	module.lcl++
	module.reqTs = module.lcl
	module.nbrResps = 0

	for i := 0; i < len(module.processes); i++ {
		if i != module.id {
			module.sendToLink(module.processes[i], fmt.Sprintf("reqEntry;%d;%d", module.id, module.lcl), module.processes[module.id])
		}
	}
	module.st = wantMX
}

func (module *DIMEX_Module) handleUponReqExit() {
	for i, isWaiting := range module.waiting {
		if isWaiting {
			module.sendToLink(module.processes[i], fmt.Sprintf("respOK;%d;%d", module.id, module.lcl), module.processes[module.id])
			fmt.Printf("Enviei OK para o processo %d\n", i)
			module.waiting[i] = false
		}
	}

	module.st = noMX
}

// ------------------------------------------------------------------------------------
// ------- tratamento de mensagens de outros processos
// ------- UPON respOk
// ------- UPON reqEntry
// ------------------------------------------------------------------------------------

func (module *DIMEX_Module) handleUponDeliverRespOk(msgOutro PP2PLink.PP2PLink_Ind_Message) {
	module.nbrResps++

	if module.nbrResps == len(module.processes)-1 {
		module.Ind <- dmxResp{}
		module.st = inMX
	}
}

func (module *DIMEX_Module) handleUponDeliverReqEntry(msgOutro PP2PLink.PP2PLink_Ind_Message) {
	payload := strings.Split(msgOutro.Message, ";")

	senderId, err := strconv.Atoi(payload[1])
	if err != nil {
		fmt.Printf("Erro ao converter senderId: %v\n", err)
		return
	}
	senderTs, err := strconv.Atoi(payload[2])
	if err != nil {
		fmt.Printf("Erro ao converter senderTs: %v\n", err)
		return
	}

	if (module.st == noMX) || (module.st == wantMX && !before(module.id, module.reqTs, senderId, senderTs)) {
		module.sendToLink(module.processes[senderId], fmt.Sprintf("respOK;%d;%d", module.id, module.reqTs), module.processes[module.id])
		fmt.Printf("Enviei OK para o %d\n", senderId)
	} else {
		module.waiting[senderId] = true
	}

	module.lcl = customMax(module.lcl, senderTs)
}

// ------------------------------------------------------------------------------------
// ------- funcoes de ajuda
// ------------------------------------------------------------------------------------

func (module *DIMEX_Module) sendToLink(address string, content string, space string) {
	module.outDbg(space + " ---->>>>   to: " + address + "     msg: " + content)
	module.Pp2plink.Req <- PP2PLink.PP2PLink_Req_Message{
		To:      address,
		Message: content}
}

func before(oneId, oneTs, othId, othTs int) bool {
	if oneTs < othTs {
		return true
	} else if oneTs > othTs {
		return false
	} else {
		return oneId < othId
	}
}

func (module *DIMEX_Module) outDbg(s string) {
	if module.dbg {
		fmt.Println(". . . . . . . . . . . . [ DIMEX : " + s + " ]")
	}
}
