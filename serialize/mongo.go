package serialize

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/ndsky1003/task/itask"
	"github.com/ndsky1003/task/taskstatus"
)

type mongo_serialize[T any] struct {
	client         *mongo.Client
	database, coll string
}

func Mongo[T any](client *mongo.Client, database, coll string) *mongo_serialize[T] {
	if client == nil {
		panic("client is nil")
	}
	if _, err := client.
		Database(database).
		Collection(coll).
		Indexes().
		CreateMany(context.Background(), []mongo.IndexModel{
			{
				Keys:    bson.D{{Key: "Status", Value: 1}, {Key: "Type", Value: 1}, {Key: "UpdateTime", Value: 1}},
				Options: options.Index().SetUnique(false).SetSparse(false),
			},
			{
				Keys:    bson.D{{Key: "Status", Value: 1}, {Key: "Type", Value: 1}, {Key: "CreateTime", Value: 1}},
				Options: options.Index().SetUnique(false).SetSparse(false),
			},
		}); err != nil {
		panic(err)
	}

	return &mongo_serialize[T]{
		client:   client,
		database: database,
		coll:     coll,
	}
}

func (this *mongo_serialize[T]) Init() error {
	return nil
}

func (this *mongo_serialize[T]) Add(task itask.ITask) error {
	_, err := this.client.
		Database(this.database).
		Collection(this.coll).
		InsertOne(context.Background(), task)
	return err
}

func (this *mongo_serialize[T]) Next(exclude_t ...uint32) (itask.ITask, error) {
	filter := bson.M{
		"Status": taskstatus.Init,
	}
	if len(exclude_t) > 0 {
		arr := make([]uint32, len(exclude_t))
		copy(arr, exclude_t)
		filter["Type"] = bson.M{"$nin": arr}
	}

	r := this.client.
		Database(this.database).
		Collection(this.coll).
		FindOneAndUpdate(context.Background(),
			filter,
			bson.M{"$set": bson.M{"UpdateTime": time.Now()}},
			options.FindOneAndUpdate().SetSort(bson.M{"UpdateTime": 1}),
		)

	var doc T
	if err := r.Decode(doc); err != nil {
		return nil, err
	}
	var d any = doc
	if v, ok := d.(itask.ITask); ok {
		return v, nil
	} else {
		return nil, errors.New("type doc is not implement ITask")
	}
}

func (this *mongo_serialize[T]) NextByType(t uint32) (itask.ITask, error) {
	filter := bson.M{
		"Status": taskstatus.Init,
		"Type":   t,
	}
	r := this.client.
		Database(this.database).
		Collection(this.coll).
		FindOne(context.Background(),
			filter,
			options.FindOne().SetSort(bson.M{"CreateTime": 1}),
		)

	var doc T
	if err := r.Decode(doc); err != nil {
		return nil, err
	}
	var d any = doc
	if v, ok := d.(itask.ITask); ok {
		return v, nil
	} else {
		return nil, errors.New("type doc is not implement ITask")
	}
}

func (this *mongo_serialize[T]) HasNext(exclude_t ...uint32) (bool, error) {
	filter := bson.M{
		"Status": taskstatus.Init,
	}
	if len(exclude_t) > 0 {
		arr := make([]uint32, len(exclude_t))
		copy(arr, exclude_t)
		filter["Type"] = bson.M{"$nin": arr}
	}
	count, err := this.client.
		Database(this.database).
		Collection(this.coll).
		CountDocuments(
			context.Background(),
			filter,
		)
	return count > 0, err
}

func (this *mongo_serialize[T]) Remove(t itask.ITask) error {
	_, err := this.client.
		Database(this.database).
		Collection(this.coll).
		DeleteOne(context.Background(), t.GetID())
	return err
}

// 任务状态还原。eg:如果任务出错，需要将状态还原，等待下次执行
func (this *mongo_serialize[T]) UpdateStatus2Init(t itask.ITask) error {
	_, err := this.client.
		Database(this.database).
		Collection(this.coll).
		UpdateByID(context.Background(),
			t.GetID(),
			bson.M{"$set": bson.M{"Status": taskstatus.Init}},
		)
	return err
}
